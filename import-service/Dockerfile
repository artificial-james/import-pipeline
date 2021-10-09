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
COPY ./import-service import-service

RUN go test ./import-service/...
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ./bin/import-service ./import-service/

FROM busybox

COPY --from=build_image /app/bin/import-service /usr/local/bin/
CMD import-service
