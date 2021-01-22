package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
)

type json_response struct {
	Status string `json:"status"`
	Data   data   `json:"data"`
}

type data struct {
	Results []results `json:"result"`
}

type results struct {
	Metric metrics       `json:"metric"`
	Value  []interface{} `json:"value"` // todo: Not sure about this, value is an size 2 array with a float and a string
}

type metrics struct {
	Name     string
	Group    string
	Instance string
	Job      string
}

func UnmarshalJSON(jsonBlob []byte, json_resp *json_response) error {
	err := json.Unmarshal(jsonBlob, &json_resp)
	if err != nil {
		stdlog.Fatal(err)
	}
	return err
}

type fakeStorage struct {
	// Nothing needed here for now
}

func (s *fakeStorage) Appender(ctx context.Context) storage.Appender {
	return &fakeAppender{}
}

type fakeAppender struct {
	// Nothing needed here for now
}

func (a *fakeAppender) Add(l labels.Labels, t int64, v float64) (uint64, error) {
	fmt.Println("Add", l, t, v)
	return 0, nil
}

func (a *fakeAppender) AddFast(ref uint64, t int64, v float64) error {
	fmt.Println(ref, t, v)
	return nil
}

func (a *fakeAppender) Commit() error {
	return nil
}

func (a *fakeAppender) Rollback() error {
	return nil
}

func QueryFunc(ctx context.Context, q string, t time.Time) (promql.Vector, error) {
	target := "http://localhost:9090/api/v1/query?query=" + q
	res, err := http.Get(target) //address of local prometheus instance
	if err != nil {
		stdlog.Fatal(err)
	}
	resp_text, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		stdlog.Fatal(err)
	}
	var json_resp json_response
	UnmarshalJSON(resp_text, &json_resp)

	v := promql.Vector{}
	for _, result := range json_resp.Data.Results {
		s := promql.Sample{
			Point:  promql.Point{T: 0, V: result.Value[0].(float64)},      //todo: fix T to pass Value[1]
			Metric: labels.FromStrings(result.Metric.Name, "b", "c", "d")} //todo: use correct labls
		fmt.Println(s)
		v = append(v, s)
	}

	return v, nil
}

func Queryable(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	return storage.NoopQuerier(), nil
}

func main() {
	destination := fakeStorage{}
	logger := log.NewLogfmtLogger(os.Stdout)
	manager_options := &rules.ManagerOptions{
		ExternalURL: &url.URL{},
		QueryFunc:   QueryFunc,
		Context:     context.Background(),
		Appendable:  &destination,
		Queryable:   storage.QueryableFunc(Queryable),
		Logger:      logger,
	}
	manager := rules.NewManager(manager_options)
	err := manager.Update(time.Second, []string{"/Users/maxamin/Downloads/prometheus-2.24.0.darwin-amd64/prometheus.rules.yml"}, nil)
	if err != nil {
		stdlog.Fatal(err)
	}
	manager.Run()
}
