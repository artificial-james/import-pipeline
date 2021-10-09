FROM golang as build_image
# ARG builduser
# ARG buildtokenn

# RUN go env -w GOPRIVATE=github.com/artificial-james
# RUN git config --global url."https://$builduser:$buildtoken@github.com/artificial-james".insteadOf "https://github.com/artificial-james"

WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY ./protos/go protos/go
COPY ./resource-service resource-service

RUN go test ./resource-service/...
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ./bin/resource-service ./resource-service/

FROM busybox

COPY --from=build_image /app/bin/resource-service /usr/local/bin/
CMD resource-service
