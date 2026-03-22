FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /fn
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o function .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /fn/function /function
EXPOSE 9443
USER nonroot:nonroot
ENTRYPOINT ["/function"]
