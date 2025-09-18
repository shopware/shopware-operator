FROM debian:stable-slim

ARG ARCH

# https://dev.mysql.com/get/Downloads/MySQL-Shell/mysql-shell-8.4.6-linux-glibc2.28-arm-64bit.tar.gz
# https://dev.mysql.com/get/Downloads/MySQL-Shell/mysql-shell-8.4.6-linux-glibc2.28-x86-64bit.tar.gz

# Dependencies for MySQL Shell
RUN apt-get update && apt-get install -y \
  wget libncurses6 libtinfo6 libstdc++6 libssl3 \
  libuuid1 libkeyutils1 krb5-multidev \
  && rm -rf /var/lib/apt/lists/*

# Download MySQL Shell (choose x86-64bit or aarch64 tarball)
RUN wget -q https://dev.mysql.com/get/Downloads/MySQL-Shell/mysql-shell-8.4.6-linux-glibc2.28-${ARCH}bit.tar.gz \
  && mkdir -p /opt/mysqlsh \
  && tar -xzf mysql-shell-*.tar.gz -C /opt/mysqlsh --strip-components=1 \
  && rm mysql-shell-*.tar.gz

COPY snapshot /
COPY LICENSE /LICENSE
COPY third-party-licenses.md /third-party-licenses.md

#ENTRYPOINT ["/opt/mysqlsh/bin/mysqlsh"]
ENTRYPOINT ["/snapshot"]
CMD ["--help"]

