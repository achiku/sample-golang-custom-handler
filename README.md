# sample-golang-custom-handler
Sample custom handler


###### simple select with begin/commit

```
$ wrk http://localhost:8991/api/select/tran
Running 10s test @ http://localhost:8991/api/select/tran
  2 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    14.51ms   21.99ms 178.52ms   87.80%
    Req/Sec     0.88k   344.06     1.80k    64.65%
  17471 requests in 10.05s, 2.98MB read
Requests/sec:   1738.60
Transfer/sec:    303.72KB
```


###### simple select without begin/commit
```
$ wrk http://localhost:8991/api/select/notran
Running 10s test @ http://localhost:8991/api/select/notran
  2 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    16.60ms   25.87ms 231.67ms   86.74%
    Req/Sec     1.11k   594.24     2.90k    65.15%
  22231 requests in 10.07s, 3.79MB read
Requests/sec:   2207.86
Transfer/sec:    385.71KB
```
