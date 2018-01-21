# NOTE: dcos-commons uses ubuntu:16.04, dcos networking generally uses debian:9.0
FROM debian:9.0

ENV HOMEDIR /root
ENV GOROOT /usr/local/go
ENV GOPATH /go-tmp
ENV BUILDDIR /build
ENV PROJECTDIR $GOPATH/src/github.com/mesosphere/dcos-edge-lb
# SSHDIR and AWSDIR should be set via volume during docker run
ENV SSHDIR ${HOMEDIR}/.ssh
ENV AWSDIR ${HOMEDIR}/.aws
ENV DOCKER_USERNAME nobody
ENV DOCKER_PASSWORD insecure
ENV PATH .:$GOPATH/bin:$GOROOT/bin:$PATH
ENV SSH_GIT_KEY id_rsa
# HACK: from dcos-commons, see HACK nearer to apt package listing
# shakedown and dcos-cli require this to output cleanly
ENV LC_ALL=C.UTF-8 LANG=C.UTF-8

WORKDIR ${BUILDDIR}

COPY ./ ${PROJECTDIR}

# avoid docker in docker, see https://jpetazzo.github.io/2015/09/03/do-not-use-docker-in-docker-for-ci/
# when running, include the option `-v /var/run/docker.sock:/var/run/docker.sock` .

# HACK: from jq through the end of the apt-get install packages, these are from dcos-commons, see
# https://github.com/mesosphere/dcos-commons/blob/master/Dockerfile , BUT ideally we shouldn't be
# ensuring dcos-commons environment w/i this container, but instead executing a dockerized build
# and (DC/OS package) publish mechanism from dcos-commons.

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ssh \
        git \
        wget \
        curl \
        apt-transport-https \
        ca-certificates \
        software-properties-common \
        build-essential \
        libssl-dev \
        libffi-dev \
        python-dev \
        python3 \
        python3-pip \
        groff \
        gnupg2 \
        jq \
        default-jdk \
        tox \
        zip && \
    [ -f /usr/local/bin/python ] || ln -s $(which python3) /usr/local/bin/python && \
    [ -f /usr/local/bin/pip ] || ln -s $(which pip3) /usr/local/bin/pip && \
    pip install setuptools --upgrade && \
    pip install wheel && \
    pip install awscli && \
    wget --no-check-certificate https://storage.googleapis.com/golang/go1.9.1.linux-amd64.tar.gz && \
    tar -xzf go1.9.1.linux-amd64.tar.gz && \
    mv go /usr/local && \
    curl -fsSL get.docker.com -o get-docker.sh && \
    sh get-docker.sh && \
    ln -s $PROJECTDIR/scripts/ci-build.sh $BUILDDIR/ci-build

ENTRYPOINT []
CMD []
