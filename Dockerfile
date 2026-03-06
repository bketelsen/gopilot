FROM golang:1.25 AS build
WORKDIR /src
RUN sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN task build

FROM gcr.io/distroless/static-debian12
COPY --from=build /src/gopilot /usr/local/bin/gopilot
ENTRYPOINT ["gopilot"]
