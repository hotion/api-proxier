# Commnads declare
GOCMD=go
GOTEST=$(GOCMD) test
GOBUILD=$(GOCMD) build

# Params define
MAIN_PATH=./cmd
PACKAGE_PATH=package
PACKAGE_BIN_PATH=package/bin
BIN=apiproxier
FILENAME=apiproxier-linux-adm64.tar.gz
GITCOMMIT=`git rev-parse HEAD`
GITBRANCH=`git symbolic-ref --short -q HEAD`
BUILD_TIME=`date +%FT%T%z`

default: clean build archive

test: 
	- $(GOTEST) ./... -v

build:
	# building
	- mkdir $(PACKAGE_PATH)
	- mkdir $(PACKAGE_BIN_PATH)
	cd $(MAIN_PATH) && CGO_ENABLE=false GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BIN)
	echo "branch=${GITBRANCH} commitID=${GITCOMMIT} buildTime=${BUILD_TIME}" > VERSION
	mv VERSION ${PACKAGE_PATH}
	mv "$(MAIN_PATH)/$(BIN)" $(PACKAGE_BIN_PATH)

archive: 
	# packing
	cd $(PACKAGE_PATH) && tar -zcvf $(FILENAME) ./*

clean:
	# cleaning
	rm -fr $(PACKAGE_PATH)
	rm -fr $(FILENAME)