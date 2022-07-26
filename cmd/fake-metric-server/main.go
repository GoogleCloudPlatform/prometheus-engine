// Copyright 2022 Google LLC
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

package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/e2e"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/grpc"
)

var (
	maxTimeSeriesPerRequest = flag.Int("max-time-series-per-request", 200,
		"The maximum amount of time series we can send per CreateTimeSeries. Default is 200.")
	port = flag.Int("port", 5678, "The port to listen for requests on.")
)

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Println("Listening on " + fmt.Sprintf("0.0.0.0:%d", *port))
	serv := grpc.NewServer()
	monitoringpb.RegisterMetricServiceServer(serv, e2e.NewFakeMetricServer(*maxTimeSeriesPerRequest))
	if err := serv.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
