FROM golang:1.20

WORKDIR /srv/root

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . /srv/root

RUN apt install -y python3-pip

RUN go build

EXPOSE 80

CMD ["./scripts/start.sh"]
