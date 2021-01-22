package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/gpe-collector/pkg/export"
	"github.com/pkg/errors"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
)

type jsonResponse struct {
	Status string `json:"status"`
	Data   data   `json:"data"`
}

type vector []sample

type data struct {
	Results vector `json:"result"`
}

type sample struct {
	Value  point         `json:"value"`
	Metric labels.Labels `json:"metric"`
}

type point struct {
	T int64
	V float64
}

func (p *point) UnmarshalJSON(b []byte) error {
	// input format b = [float64,string]
	// output format p = [int64,float64]
	s := string(b)
	if s[0] != '[' || s[len(s)-1] != ']' {
		return errors.Errorf("Missing open or close bracket", s)
	}
	s = s[1 : len(s)-1] // remove brackets
	m := strings.Split(s, ",")
	if len(m) != 2 {
		return errors.Errorf("Expected two values, recieved %d value(s)", len(m))
	}
	T, err := strconv.ParseFloat(m[0], 64)
	if err != nil {
		return err
	}
	p.T = int64(T * 1000)
	V, err := strconv.Unquote(m[1])
	if err != nil {
		return err
	}
	p.V, err = strconv.ParseFloat(V, 64)
	if err != nil {
		return err
	}
	return err
}

func QueryFunc(ctx context.Context, q string, t time.Time) (promql.Vector, error) {
	path := url.QueryEscape(q)
	target := "http://localhost:9090/api/v1/query?query=" + path
	res, err := http.Get(target)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	respText, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var jsonResp jsonResponse
	if err := json.Unmarshal(respText, &jsonResp); err != nil {
		return nil, err
	}

	//TODO: this block is only necessary to make a vector of type promql.Vector, should be fixed by moving the methods to value.go
	v := make(promql.Vector, len(jsonResp.Data.Results))
	for i, result := range jsonResp.Data.Results {
		s := promql.Sample{
			Point:  promql.Point(result.Value),
			Metric: result.Metric,
		}
		v[i] = s
	}
	return v, err
}

func main() {
	logger := log.NewLogfmtLogger(os.Stderr)

	a := kingpin.New("rule", "The Prometheus Rule Evaluator")
	exporterOptions := export.NewFlagOptions(a)

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "Error parsing commandline arguments"))
		a.Usage(os.Args[1:])
		os.Exit(2)
	}

	destination, err := export.NewStorage(logger, nil, *exporterOptions)
	if err != nil {
		logger.Log("msg", "Creating a Cloud Monitoring Exporter failed", "err", err)
		os.Exit(1)
	}

	noopQueryable := func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
		return storage.NoopQuerier(), nil
	}

	managerOptions := &rules.ManagerOptions{
		ExternalURL: &url.URL{},
		QueryFunc:   QueryFunc,
		Context:     context.Background(),
		Appendable:  destination,
		Queryable:   storage.QueryableFunc(noopQueryable),
		Logger:      logger,
	}

	manager := rules.NewManager(managerOptions)
	err = manager.Update(time.Second*10, []string{"experimental/prometheus.rules.yml"}, nil)
	if err != nil {
		logger.Log("msg", "Updating rule manager failed", "err", err)
		os.Exit(1)
	}
	go func() {
		err := destination.Run(context.Background())
		if err != nil {
			logger.Log("msg", "Background processing of storage failed", "err", err)
			os.Exit(1)
		}
	}()
	manager.Run()
}
