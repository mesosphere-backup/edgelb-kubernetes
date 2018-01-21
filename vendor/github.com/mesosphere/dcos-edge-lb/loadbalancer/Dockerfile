FROM debian:9.0

ENV LBWORKDIR /dcosfiles
WORKDIR ${LBWORKDIR}

# Runtime Dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        inetutils-syslogd \
        iptables \
        liblua5.3-0 \
        libssl1.0.2 \
        openssl \
        procps \
        python3 \
        runit \
        socat \
        wget

# Build / Setup HAProxy, Python
COPY requirements.txt ${LBWORKDIR}
COPY build-haproxy.sh ${LBWORKDIR}
RUN ${LBWORKDIR}/build-haproxy.sh

# Cleanup
RUN rm -rf /var/lib/apt/lists/*

# Copy Runtime Files
COPY containerfiles/ ${LBWORKDIR}/

# Set empty for dcos-commons
ENTRYPOINT []
CMD []