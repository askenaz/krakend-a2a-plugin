# golang:1.26.8-alpine3.23
FROM golang@sha256:a8fa79c5bd40d880b52bd3b6d7669ecdcfd00e85facdd427d279efb5ddd79cb1 AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -o /mock-agent .

FROM scratch
LABEL maintainer="Autores del plugin A2A para KrakenD"
COPY --from=build /mock-agent /mock-agent
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/mock-agent"]
