# CAP: HTTP from Scratch

This is the base for the training "HTTP from Scratch".
Purpose of this training is to deepen Go development and understanding of the HTTP protocol.

## Intro

The HyperText Transfer Protocol (HTTP) is the basis for the „Internet“. 

In this training, we'll implement the still widely supported and used version 1.1, e.g.

```http
GET /index.html HTTP/1.1
Host: www.example.com
User-Agent: Browser
```

returns

```
HTTP/1.1 200 OK
Server: nginx/1.15.8
Date: Fri, 09 Apr 2021 13:34:03 GMT
Content-Type: text/html
Content-Length: 235
<html>
  <head>
  <title>Hello World</title>
</head>
<body>
  Hello World
</body>
</html>
```

## The Task (Day 1)

* Read [the RFC](https://www.rfc-editor.org/info/rfc9112/)
* Implement a basic HTTP server that
  * can handle client connections
  * parse requests
  * write responses

Until this point, it is fine to use the http.DefaultServeMux.



## To run Victoria Metrics
docker run --rm -p 8428:8428 \
  -v "$PWD/vmscrape.yml:/etc/vm/scrape.yml" \
  victoriametrics/victoria-metrics \
  -promscrape.config=/etc/vm/scrape.yml


curl -s 'http://localhost:8428/api/v1/query?query=http_requests_total' | jq .


## How to run observability stack with k8s

```bash

# 1. build the image
make image
docker images httpserver:dev            # expect: httpserver:dev  ~18MB

# 2. deploy the server (creates cluster + ingress on first run)
make deploy-local
kubectl get pods                         # expect: 2x httpserver pods  Running
curl -s -o /dev/null -w "%{http_code}\n" http://localhost/healthz   # expect: 200

# 3. install the metrics stack
make vmstack
kubectl -n monitoring get pods           # expect: operator, vmsingle, vmagent  Running

# 4. turn on the scrape
make scrape
kubectl get vmservicescrape              # expect: httpserver

make clean        # deletes the kind cluster and everything in it
kind get clusters # expect: No kind clusters found.

```
