package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"cloud.google.com/go/vertexai/genai"
	"github.com/prometheus/alertmanager/api/v2/models"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	alertsKey = new(contextKey)
	eventsKey = new(contextKey)
	logsKey   = new(contextKey)
	hintsKey  = new(contextKey)
)

const (
	promptTemplate = `Consider the following dump of Kubernetes logs, events, and other resources, delimited by "---"s:
	---
	%s
	---
	Can you give a summary of any issues and suggestions to fix in 500 words or less in this language: %s?`
)

// contextKey is a concrete struct (as opposed to string or another built-in).
// See https://godoc.org/context#WithValue for an explanation.
type contextKey struct{ bool }

func with(r *http.Request, key *contextKey, value interface{}) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), key, value))
}

func get(r *http.Request, key *contextKey) interface{} {
	return r.Context().Value(key)
}

func respond(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("done"))
}

func extractAlertPayload(next http.Handler) http.Handler {
	var fn = func(w http.ResponseWriter, r *http.Request) {
		log.Printf("extracting alert payload")
		var alerts models.PostableAlerts

		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&alerts)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("decode alerts. err: %s", err)))
			return
		}

		r = with(r, alertsKey, alerts)
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func getEvents(kclient kubernetes.Interface) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		var fn = func(w http.ResponseWriter, r *http.Request) {
			alerts := get(r, alertsKey).(models.PostableAlerts)
			// Assume just one alert to consider for now.
			alert := alerts[0]
			var events *corev1.EventList
			var err error
			if namespace, ok := alert.Labels["namespace"]; ok {
				events, err = kclient.CoreV1().Events(namespace).List(r.Context(), v1.ListOptions{})
				if err != nil {
					fmt.Printf("getting events for namespace %s: %s", namespace, err)
					return
				}
			}
			r = with(r, eventsKey, events)
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

func getLogs(kclient kubernetes.Interface) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		var fn = func(w http.ResponseWriter, r *http.Request) {
			alerts := get(r, alertsKey).(models.PostableAlerts)
			// Assume just one alert to consider for now.
			alert := alerts[0]
			var logs []string
			var lines int64 = 10
			if pod, ok := alert.Labels["pod"]; ok {
				req := kclient.CoreV1().Pods("default").GetLogs(pod, &corev1.PodLogOptions{TailLines: &lines})
				stream, err := req.Stream(r.Context())
				defer stream.Close()
				if err != nil {
					fmt.Printf("getting logs for pod %s: %s", pod, err)
					return
				}
				buf := make([]byte, 2000) // Buffer for reading logs
				for {
					numBytes, err := stream.Read(buf)
					if err != nil {
						break // End of logs
					}
					logs = append(logs, string(buf[:numBytes]))
					fmt.Printf("logs: %s", logs)
				}
			}
			r = with(r, logsKey, logs)
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

func sendAlert(alertmanagerURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		var fn = func(w http.ResponseWriter, r *http.Request) {
			alerts := get(r, alertsKey).(models.PostableAlerts)
			payload, err := json.Marshal(alerts)
			if err != nil {
				fmt.Printf("marshalling alerts: %s", err)
				return
			}
			client := &http.Client{}
			req, err := http.NewRequest("POST", alertmanagerURL, bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")

			if err != nil {
				fmt.Printf("creating request: %s", err)
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("calling alertmanager: %s", err)
				return
			}
			if resp.StatusCode != http.StatusOK {
				fmt.Printf("alertmanager response: %d", resp.StatusCode)
				// Read and print the response body
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					fmt.Println("Error reading response body:", err)
					return
				}
				bodyString := string(bodyBytes)
				fmt.Println("Response Body:", bodyString)
			}

			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

func attachHints(next http.Handler) http.Handler {
	var fn = func(w http.ResponseWriter, r *http.Request) {
		hints := get(r, hintsKey).(string)
		alerts := get(r, alertsKey).(models.PostableAlerts)
		// Assume one alert for now.
		alert := alerts[0]
		alert.Annotations["hint"] = hints
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func callAI(projectID, region, model, language string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		var fn = func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			client, err := genai.NewClient(ctx, projectID, region)
			if err != nil {
				w.Write([]byte(fmt.Sprintf("initializing chat client. err: %s", err)))
				return
			}

			// Create prompt from events and logs.
			alerts := get(r, alertsKey).(models.PostableAlerts)
			// Assume one alert for now.
			alert := alerts[0]
			events := get(r, eventsKey).(*corev1.EventList)
			logs := get(r, logsKey)
			eventsAndLogs := fmt.Sprintf("---alert details---\n%s\n---events---\n%s\n---logs---\n%s", alert, events, logs)
			prompt := fmt.Sprintf(promptTemplate, eventsAndLogs, language)

			gemini := client.GenerativeModel(model)
			chat := gemini.StartChat()

			// Send prompt to backend.
			//log.Printf("calling AI with prompt:\n%s\n", prompt)
			resp, err := chat.SendMessage(ctx, genai.Text(prompt))
			if err != nil {
				fmt.Printf("calling SendMessage: %s\nexiting.", err)
				return
			}
			hint := resp.Candidates[0].Content.Parts[0]
			r = with(r, hintsKey, fmt.Sprintf("%s", hint))
			fmt.Println(hint)

			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}
