FROM debian:9.0

ENV LBWORKDIR /dcosfiles
ENV LBTMPDIR /dcosfilestmp

WORKDIR ${LBTMPDIR}

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        iptables \
        openssl \
        inetutils-syslogd \
        procps \
        runit \
        socat \
        jq \
        curl

COPY containerfiles/ ${LBTMPDIR}/

ENTRYPOINT []
CMD []