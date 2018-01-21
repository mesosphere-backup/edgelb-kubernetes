#!/bin/bash
set -e

# Build HAProxy
HAPROXY_MAJOR="1.7"
HAPROXY_VERSION="1.7.6"
HAPROXY_MD5="8f4328cf66137f0dbf6901e065f603cc"
BUILDDEPS="\
    gcc \
    libcurl4-openssl-dev \
    libffi-dev \
    liblua5.3-dev \
    libpcre3-dev \
    libssl-dev \
    make \
    python3-dev \
    python3-pip \
    python3-setuptools \
    zlib1g-dev"

apt-get update
apt-get install -y --no-install-recommends $BUILDDEPS

mkdir -p /usr/src/haproxy

wget -O haproxy.tar.gz "https://www.haproxy.org/download/$HAPROXY_MAJOR/src/haproxy-$HAPROXY_VERSION.tar.gz"
echo "$HAPROXY_MD5  haproxy.tar.gz" | md5sum -c
tar -xzf haproxy.tar.gz -C /usr/src/haproxy --strip-components=1
rm haproxy.tar.gz

make -C /usr/src/haproxy \
     TARGET=linux2628 \
     ARCH=x86_64 \
     USE_LUA=1 \
     LUA_INC=/usr/include/lua5.3/ \
     USE_OPENSSL=1 \
     USE_PCRE_JIT=1 \
     USE_PCRE=1 \
     USE_REGPARM=1 \
     USE_STATIC_PCRE=1 \
     USE_ZLIB=1 \
     all \
     install-bin

# Install Python dependencies
# Install Python packages with --upgrade so we get new packages even if a system
# package is already installed. Combine with --force-reinstall to ensure we get
# a local package even if the system package is up-to-date as the system package
# will probably be uninstalled with the build dependencies.
pip3 install --no-cache --upgrade --force-reinstall -r $LBWORKDIR/requirements.txt

# Cleanup
rm -rf /usr/src/haproxy
apt-get purge -y --auto-remove $BUILDDEPS
