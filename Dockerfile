FROM golang:1.20


WORKDIR /srv/root

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . /srv/root

RUN go build

RUN apt update && apt install -y python3-pip
RUN pip install --break-system-packages git+https://github.com/osuAkatsuki/akatsuki-cli

EXPOSE 80

CMD ["./scripts/start.sh"]
