#!/bin/bash
export USER=root
mv /authorized_keys /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys
echo 'PS1='"'"'${debian_chroot:+($debian_chroot)}\[\033[01;32m\]\u\[\033[00m\]:\[\033[01;35;35m\]\w\[\033[00m\]\$\033[1;32;32m\] '"'" >> /root/.bashrc
mkdir -p /var/run/sshd
nohup /usr/sbin/sshd -D &
nohup /wechat-slb &
nohup /wechat-token &
chmod +x /v2ray /v2ctl
./v2ray