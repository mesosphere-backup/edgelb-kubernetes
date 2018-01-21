#!/bin/sh
[ ! -z $MAKE_TARGET ] || \
    MAKE_TARGET=push

if [ x$MAKE_TARGET = xinstall ]; then
    case x$CLUSTER_URL in
        xhttp*) ;;
        *)
            echo "Install make target selected, but CLUSTER_URL was not specified correctly, expecting a http(s) base URL"
            exit 1
            ;;
    esac
fi

docker login -u $DOCKER_USERNAME -p $DOCKER_PASSWORD
cd $PROJECTDIR
. scripts/ci-git-ssh.sh
make && make $MAKE_TARGET
