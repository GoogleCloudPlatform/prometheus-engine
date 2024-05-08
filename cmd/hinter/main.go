package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	projectID = flag.String("project-id", "test-project", "Project ID where Vertex AI is being called.")
	region    = flag.String("region", "test-location", "Location where Vertex AI is being called.")
	model     = flag.String("model", "gemini-1.0-pro-vision", "Vertex AI model to call.")
	language  = flag.String("language", "english", "Language to use for prompt responses.")
)

func main() {
	flag.Parse()

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
	if err != nil {
		log.Fatalf("initializing kube clientconfig: %s", err)
	}
	kubeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("creating kubeclient: %s", err)
	}
	r := chi.NewRouter()
	r.Use(extractAlertPayload,
		getEvents(kubeClient),
		//getLogs(kubeClient),
		callAI(*projectID, *region, *model, *language),
		attachHints,
		sendAlert("http://localhost:9093/api/v2/alerts"),
	)
	r.Post("/api/v2/alerts", respond)

	log.Print("now serving...")
	http.ListenAndServe(":8080", r)
}
