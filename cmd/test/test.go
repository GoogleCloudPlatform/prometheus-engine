package main

import (
	"context"
	"fmt"
	"os"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	"google.golang.org/api/option"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

func main() {
	//var opts []grpc.DialOption
	conn, _ := grpc.Dial("localhost:8080", grpc.WithTransportCredentials(insecure.NewCredentials()))
	client, err := gcm.NewMetricClient(context.TODO(), option.WithGRPCConn(conn))
	println("hi")
	if err != nil {
		print(err.Error())
		os.Exit(1)
	}
	println("hi")
	err2 := client.CreateTimeSeries(context.TODO(), &monitoringpb.CreateTimeSeriesRequest{
		Name: "projectName",
		TimeSeries: []*monitoringpb.TimeSeries{{
			Resource: &monitoredrespb.MonitoredResource{
				Type: "prometheus_target",
				Labels: map[string]string{
					"project_id": "example-project",
					"location":   "europe",
					"cluster":    "foo-cluster",
					"namespace":  "",
					"job":        "job1",
					"instance":   "instance1",
				},
			},
			Metric: &metricpb.Metric{
				Type:   "prometheus.googleapis.com/metric1/gauge",
				Labels: map[string]string{"k1": "v1"},
			},
			MetricKind: metricpb.MetricDescriptor_GAUGE,
			ValueType:  metricpb.MetricDescriptor_DOUBLE,
			Points: []*monitoringpb.Point{{
				Interval: &monitoringpb.TimeInterval{
					StartTime: &timestamppb.Timestamp{Seconds: 3},
					EndTime:   &timestamppb.Timestamp{Seconds: 4},
				},
				Value: &monitoringpb.TypedValue{
					Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 0.6},
				},
			}},
		}},
	})
	if err2 == nil {
		println("yay")
	} else {
		println(err2.Error())
		os.Exit(1)
	}
	timeSeriesIterator := client.ListTimeSeries(context.TODO(), &monitoringpb.ListTimeSeriesRequest{
		Name:   "projectName",
		Filter: "metric.type = prometheus.googleapis.com/metric1/gauge",
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamppb.Timestamp{Seconds: 0},
			EndTime:   &timestamppb.Timestamp{Seconds: 5},
		},
	})
	timeSeries, _ := timeSeriesIterator.Next()
	if timeSeries != nil {
		fmt.Printf("%+v \n", timeSeries)
		timeSeries, _ = timeSeriesIterator.Next()
	}
}
