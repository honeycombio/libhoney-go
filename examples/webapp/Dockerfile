FROM golang:alpine

WORKDIR /app

COPY . ./

RUN go mod download
RUN go build -o ./shoutr

RUN apk add --update --no-cache ca-certificates
ENTRYPOINT ["./shoutr"]
