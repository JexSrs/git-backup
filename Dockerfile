FROM golang:1.20-alpine

WORKDIR "/app"

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY src ./src/

# Compile
RUN mkdir bin
RUN go build -o bin main/src/...

# Run
CMD [ "./bin/src" ]