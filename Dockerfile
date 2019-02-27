FROM golang:latest

WORKDIR /go/src/unrustlelogs

COPY . .

RUN go get -v ./...
RUN go install -v ./...

CMD ["unrustlelogs"]