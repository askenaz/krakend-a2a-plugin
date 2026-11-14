# golang:1.26.8-alpine3.23
FROM golang@sha256:a8fa79c5bd40d880b52bd3b6d7669ecdcfd00e85facdd427d279efb5ddd79cb1 AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -o /demo-secrets .

# alpine:3.23
FROM alpine@sha256:469c9ea410b9215eaee7976ba6c0639039b444be8e5d638f7abbada189971eb2
LABEL maintainer="Autores del plugin A2A para KrakenD"
COPY --from=build /demo-secrets /usr/local/bin/demo-secrets
USER 65534:65534
ENTRYPOINT ["/usr/local/bin/demo-secrets"]
CMD ["/out"]
