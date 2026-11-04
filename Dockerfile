FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build
WORKDIR /work
COPY . .
WORKDIR /work/src
RUN go test -count=1 ./... && go vet ./...
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false -ldflags="-s -w" -o /pocket .
FROM alpine:3.23@sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63
RUN apk add --no-cache ca-certificates
COPY --from=build /pocket /usr/local/bin/pocket
USER 65532:65532
EXPOSE 10000
ENTRYPOINT ["/usr/local/bin/pocket"]
