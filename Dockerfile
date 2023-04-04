FROM golang:1.20

WORKDIR /srv/root

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . /srv/root

RUN go build

EXPOSE 80

CMD ["./akatsuki-api"]