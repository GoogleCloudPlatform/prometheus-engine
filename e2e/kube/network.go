// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package e2e contains tests that validate the behavior of gmp-operator against a cluster.
// To make tests simple and fast, the test suite runs the operator internally. The CRDs
// are expected to be installed out of band (along with the operator deployment itself in
// a real world setup).
package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// wrappedConn simply wraps a net.Conn with an additional close function.
type wrappedConn struct {
	net.Conn
	closeFn func()
}

func (c *wrappedConn) Close() error {
	err := c.Conn.Close()
	c.closeFn()
	return err
}

// PortForwardClient returns a client that ports-forward all Kubernetes-local HTTP requests to the host.
func PortForwardClient(restConfig *rest.Config, kubeClient client.Client, out, errOut io.Writer) (*http.Client, error) {
	restClient, err := rest.RESTClientFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create REST client: %w", err)
	}

	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				if network != "tcp" {
					return nil, fmt.Errorf("network %q unsupported", network)
				}
				addr, err := net.ResolveTCPAddr(network, address)
				if err != nil {
					return nil, fmt.Errorf("unable to resolve TCP addr: %w", err)
				}

				pod, container, err := podByAddr(ctx, kubeClient, addr)
				if err != nil {
					return nil, fmt.Errorf("unable to get pod from IP %s: %w", addr.IP, err)
				}
				if err := waitForPodContainerReady(ctx, kubeClient, pod, container); err != nil {
					return nil, fmt.Errorf("failed waiting for pod from IP %s: %w", addr.IP, err)
				}
				resourceURL := restClient.
					Post().
					Resource("pods").
					Namespace(pod.GetNamespace()).
					Name(pod.GetName()).
					SubResource("portforward").
					URL()

				transport, upgrader, err := spdy.RoundTripperFor(restConfig)
				if err != nil {
					return nil, err
				}
				client := &http.Client{
					Transport: transport,
				}

				stopCh := make(chan struct{})
				readyCh := make(chan struct{})
				errCh := make(chan error)
				forwardDialer := spdy.NewDialer(upgrader, client, http.MethodPost, resourceURL)
				forwarder, err := portforward.NewOnAddresses(
					forwardDialer,
					// Specify IPv4 address explicitly, since GitHub Actions does not support IPv6.
					[]string{"127.0.0.1"},
					// The leading colon indicates that a random port is chosen.
					[]string{fmt.Sprintf(":%d", addr.Port)},
					stopCh,
					readyCh,
					out,
					errOut,
				)
				if err != nil {
					return nil, err
				}

				go func() {
					if err := forwarder.ForwardPorts(); err != nil {
						errCh <- err
					}
					close(errCh)
				}()

				closeForwarder := func() {
					// readyCh is closed by the port-forwarder.
					close(stopCh)
				}

				select {
				case <-readyCh:
					ports, err := forwarder.GetPorts()
					if err != nil {
						return nil, err
					}
					if len(ports) != 1 {
						return nil, fmt.Errorf("expected 1 port but found %d", len(ports))
					}
					port := ports[0]

					// Pass in tcp4 to ensure we always get IPv4 and never IPv6.
					var dialer net.Dialer
					conn, err := dialer.DialContext(ctx, "tcp4", fmt.Sprintf("127.0.0.1:%d", port.Local))
					if err != nil {
						return nil, err
					}
					return &wrappedConn{
						Conn:    conn,
						closeFn: closeForwarder,
					}, nil
				case <-stopCh:
					closeForwarder()
					return nil, fmt.Errorf("port forwarding stopped unexpectedly")
				case err := <-errCh:
					closeForwarder()
					return nil, fmt.Errorf("port forwarding failed: %w", err)
				}
			},
		},
	}, nil
}

func PortForward(ctx context.Context, restConfig *rest.Config, kubeClient client.Client, address string, out, errOut io.Writer) (net.Conn, error) {
	restClient, err := rest.RESTClientFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create REST client: %w", err)
	}
	return portForward(ctx, restConfig, restClient, kubeClient, address, out, errOut)
}

func portForward(ctx context.Context, restConfig *rest.Config, restClient *rest.RESTClient, kubeClient client.Client, address string, out, errOut io.Writer) (net.Conn, error) {
	// Cannot parse a URL without a scheme.
	if !strings.HasPrefix(address, "http") {
		address = "http://" + address
	}
	url, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("unable to parse address %q: %s", address, err)
	}
	ip := net.ParseIP(url.Host)
	urlPort := url.Port()
	if urlPort == "" {
		return nil, errors.New("unknown port")
	}
	port, err := strconv.Atoi(urlPort)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %s", address)
	}

	var pod *corev1.Pod
	var containerName string
	if ip != nil {
		pod, containerName, err = podByAddr(ctx, kubeClient, &net.TCPAddr{
			IP:   ip,
			Port: port,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to get pod from IP %s: %w", url.Host, err)
		}
	} else {
		split := strings.SplitN(url.Hostname(), ".", 3)
		if len(split) != 3 || split[2] != "svc.cluster.local" {
			return nil, fmt.Errorf("invalid address format: %s", url.Host)
		}

		service := corev1.Service{
			ObjectMeta: v1.ObjectMeta{
				Name:      split[0],
				Namespace: split[1],
			},
		}
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&service), &service); err != nil {
			return nil, fmt.Errorf("unable to get service %q: %s", client.ObjectKeyFromObject(&service), err)
		}

		pods, err := podsFromSelector(ctx, kubeClient, v1.SetAsLabelSelector(labels.Set(service.Spec.Selector)))
		if err != nil {
			return nil, fmt.Errorf("unable to get pods from service %q selector: %s", client.ObjectKeyFromObject(&service), err)
		}

		if len(pods) == 0 {
			return nil, fmt.Errorf("found not pods from service %q selector: %s", client.ObjectKeyFromObject(&service), err)
		}

		for i := range pods {
			if containerIndex := podPortContainerIndex(kubeClient, &pods[i], port); containerIndex != -1 {
				pod = &pods[i]
				containerName = pods[i].Spec.Containers[containerIndex].Name
				break
			}
		}
		if containerName == "" {
			return nil, fmt.Errorf("unable to find container for port %d", port)
		}
	}

	if err := waitForPodContainerReady(ctx, kubeClient, pod, containerName); err != nil {
		return nil, fmt.Errorf("failed waiting for pod from IP %s: %w", ip, err)
	}
	resourceURL := restClient.
		Post().
		Resource("pods").
		Namespace(pod.GetNamespace()).
		Name(pod.GetName()).
		SubResource("portforward").
		URL()

	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Transport: transport,
	}

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	errCh := make(chan error)
	forwardDialer := spdy.NewDialer(upgrader, client, http.MethodPost, resourceURL)
	forwarder, err := portforward.NewOnAddresses(
		forwardDialer,
		// Specify IPv4 address explicitly, since GitHub Actions does not support IPv6.
		[]string{"127.0.0.1"},
		// The leading colon indicates that a random port is chosen.
		[]string{fmt.Sprintf(":%d", port)},
		stopCh,
		readyCh,
		out,
		errOut,
	)
	if err != nil {
		return nil, err
	}

	go func() {
		if err := forwarder.ForwardPorts(); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	// Protect against multiple closes.
	closeForwarder := sync.OnceFunc(func() {
		// readyCh is closed by the port-forwarder.
		close(stopCh)
	})

	select {
	case <-readyCh:
		ports, err := forwarder.GetPorts()
		if err != nil {
			return nil, err
		}
		if len(ports) != 1 {
			return nil, fmt.Errorf("expected 1 port but found %d", len(ports))
		}
		port := ports[0]

		// Pass in tcp4 to ensure we always get IPv4 and never IPv6.
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, "tcp4", fmt.Sprintf("127.0.0.1:%d", port.Local))
		if err != nil {
			return nil, err
		}
		return &wrappedConn{
			Conn:    conn,
			closeFn: closeForwarder,
		}, nil
	case <-stopCh:
		closeForwarder()
		return nil, fmt.Errorf("port forwarding stopped unexpectedly")
	case err := <-errCh:
		closeForwarder()
		return nil, fmt.Errorf("port forwarding failed: %w", err)
	}
}
