FROM golang:1.18-alpine3.16 as builder

WORKDIR $GOPATH/src/wechat
COPY . .

RUN apk add --no-cache git && set -x && \
    go mod init && go get -d -v
RUN CGO_ENABLED=0 GOOS=linux go build -o /wechat-slb wechat-slb.go
RUN CGO_ENABLED=0 GOOS=linux go build -o /wechat-token wechat-token.go


FROM ubuntu:latest

ENV DEBIAN_FRONTEND=noninteractive

WORKDIR /
RUN apt-get update \
  && apt-get install -y curl openssh-server zip unzip net-tools inetutils-ping iproute2 tcpdump git vim mysql-client redis-tools tmux\
  && mkdir -p /var/run/sshd \
  && sed -ri 's/^#?PermitRootLogin\s+.*/PermitRootLogin yes/' /etc/ssh/sshd_config \
  && sed -i 's/^#\s*Port.*/Port 2222/' /etc/ssh/sshd_config \
  && sed -ri 's/^#?ClientAliveInterval\s+.*/ClientAliveInterval 60/' /etc/ssh/sshd_config \
  && sed -ri 's/^#?ClientAliveCountMax\s+.*/ClientAliveCountMax 1000/' /etc/ssh/sshd_config \
  && sed -ri 's/^#?TCPKeepAlive\s+.*/TCPKeepAlive yes/' /etc/ssh/sshd_config \
  && sed -ri 's/^#?PasswordAuthentication\s+.*/PasswordAuthentication no/' /etc/ssh/sshd_config \
  && sed -ri 's/^#PubkeyAuthentication\s+.*/PubkeyAuthentication yes/' /etc/ssh/sshd_config \
  && sed -ri 's/UsePAM yes/#UsePAM yes/g' /etc/ssh/sshd_config && mkdir -p /root/.ssh  \
  && rm -rf /var/lib/apt/lists/*
COPY --from=builder /wechat-slb .
COPY --from=builder /wechat-token .
ADD . .
RUN chmod +x /entrypoint.sh /wechat-slb
ENTRYPOINT  /entrypoint.sh

EXPOSE 443 2222 80 8880