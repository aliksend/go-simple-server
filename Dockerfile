FROM golang:1.12

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY server.go ./
RUN go build -o server.run server.go

CMD ./server.run
