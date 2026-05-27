# proglog

Log Service written in Go

## Start

Example (using port 8080)

```bash
go run main.go 8080
```

## Request

### POST

```bash
curl -X POST localhost:8000 -d \
    '{"record": { "value": "TGV0J3MgR28gIzML" }}' -i
```
```http
HTTP/1.1 200 OK
Date: Wed, 27 May 2026 05:38:07 GMT
Content-Length: 13
Content-Type: text/plain; charset=utf-8

{"offset":1}
```

### GET

```
$ curl -X GET 192.168.1.13:8000 -d \
    '{"record": { "offset": 1 }}' -i
```

```http
HTTP/1.1 200 OK
Date: Wed, 27 May 2026 09:06:08 GMT
Content-Length: 51
Content-Type: text/plain; charset=utf-8

{"record":{"value":"TGV0J3MgR28gIzML","offset":0}}
```
