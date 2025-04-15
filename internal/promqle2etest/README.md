# PromQL e2e tests

This directory contains (manual for now) test suite that allows testing various Prometheus
compliance elements around different Prometheus metric cases going through various
pipelines to GCM.

## Usage

To run those tests you need:

1. Install Go and Docker if not on your machine.
2. Obtain GCM secret with the permissions to read and write to GCM for the test project of your choice. Put the JSON body into a `GCM_SECRET` envvar. 
3. Run any Go unit test in this package; make sure to adjust default test timeout too, manual timeout is set on each test (5m).
