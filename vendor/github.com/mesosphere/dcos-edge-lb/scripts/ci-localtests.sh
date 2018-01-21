#!/bin/bash

set -euo pipefail

ORIGDIR="$(pwd)"

source "$(dirname $0)/ci-lib.sh"

diff_check() {
	diff_check_helper
	[ -z "$(diff_check_helper)" ]
}

diff_check_helper() {
    git diff
}

newfile_check() {
	newfile_check_helper
	[ -z "$(newfile_check_helper)" ]
}

newfile_check_helper() {
    git ls-files --others --exclude-standard
}

run_coverage() {
    true
    #gocov test ./... -short -timeout=10m > cov.json
    #mkdir -p $TEST_REPORTS/junit && go test -v -timeout=10m ./... | go-junit-report > $TEST_REPORTS/junit/alltests.xml
    #goveralls -service=circleci -gocovdata=cov.json -repotoken=$COVERALLS_REPO_TOKEN || true
}

download() {
    # Packages only used in tests
    go get "github.com/davecgh/go-spew/spew"
    go get "github.com/sergi/go-diff/diffmatchpatch"

    # go-mesos-operator
    cd "$TMPDIR"
    curl -LO https://github.com/google/protobuf/releases/download/v3.3.0/protoc-3.3.0-linux-x86_64.zip
    unzip protoc*
    protoc --version
    cd -
    go get -u github.com/golang/protobuf/protoc-gen-go

    go get github.com/alecthomas/gometalinter
    go get github.com/axw/gocov/gocov # https://github.com/golang/go/issues/6909
    go get github.com/mattn/goveralls
    go get github.com/jstemmer/go-junit-report
    #git describe --tags |tee VERSION

    gometalinter --install
}

go version
#go get github.com/kardianos/govendor

# apiserver references this path
cd "${GOPATH}/src/github.com/mesosphere"
ln -fs "${PROJECT_DIR}/framework/edgelb" edgelb
cd -

status_line "cd $PROJECT_DIR"
cd "$PROJECT_DIR"

status_line "Downloading"
time download

status_line "Compiling"
time make go-mesos-operator clean compile

status_line "Checking that no auto-generated code changed"
diff_check
newfile_check

# only import the key if it is not already known, otherwise we get an error that halts the build
#gpg --list-keys|grep -q -e BD292F47 || \
    #  gpg --yes --batch --import build/private.key

#go install ./...
#go test -i ./...

#gox -parallel=1 -arch=amd64 -os="linux darwin windows" -output="${ARTIFACT_DIR}/{{.Dir}}-$(<VERSION)-{{.OS}}-{{.Arch}}" -ldflags="-X main.Version=$(<VERSION)"

status_line "Running linter"
time make lint

status_line "Running tests"
time make test TESTFLAGS="-v"

status_line "Running coverage"
time run_coverage

status_line "Returning to $ORIGDIR"
cd "$ORIGDIR"
