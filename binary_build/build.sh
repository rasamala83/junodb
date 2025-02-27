#! /bin/bash
#  
#  Copyright 2023 PayPal Inc.
#  
#  Licensed to the Apache Software Foundation (ASF) under one or more
#  contributor license agreements.  See the NOTICE file distributed with
#  this work for additional information regarding copyright ownership.
#  The ASF licenses this file to You under the Apache License, Version 2.0
#  (the "License"); you may not use this file except in compliance with
#  the License.  You may obtain a copy of the License at
#  
#     http://www.apache.org/licenses/LICENSE-2.0
#  
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#  
 

GO_VERSION=1.18.2

### BUILDTOP is github root dir  ####
if [[ $BUILDTOP == "" ]]; then
  echo "BUILDTOP required but not defined"
  exit
fi

: ${WORKSPACE:=$(pwd)}
: ${RELEASE_REPO_ROOT:=$BUILDTOP/release-binary}
: ${SOURCE_ROOT:=$WORKSPACE}
: ${JUNO_BUILD_NUMBER:=${FUSION_BUILD_GENERATED:=unnumbered}}

echo "RELEASE_REPO_ROOT: $RELEASE_REPO_ROOT"
echo "SOURCE_ROOT: $SOURCE_ROOT"
echo "JUNO_BUILD_NUMBER: $JUNO_BUILD_NUMBER"

# Patches in the third_party branch
if [ -f $SOURCE_ROOT/third_party/apply_patch.sh ];then
  $SOURCE_ROOT/third_party/apply_patch.sh
fi

if [[ ! -d "$RELEASE_REPO_ROOT/tool" ]]; then
  mkdir -p $RELEASE_REPO_ROOT/tool
fi

build_time=`date '+%Y-%m-%d_%I:%M:%S%p_%Z'`
code_revision=`git rev-parse --short=8 HEAD 2> /dev/null`

if [[ "$?" != "0" ]]; then
  echo 
  echo "This script needs to be run in a git repository."
  echo "    (to get git revision of the source) "
  echo " (You may modify the script to do it differently)"
  echo 
  exit
fi

if [[ -d "$RELEASE_REPO_ROOT/tool/go" ]] && [[ "$(cat $RELEASE_REPO_ROOT/tool/go/VERSION)" != "go$GO_VERSION" ]]  ; then
	rm -fr $RELEASE_REPO_ROOT/tool/go
fi

if [[ ! -d "$RELEASE_REPO_ROOT/tool/go" ]]; then
  echo "downloading go$GO_VERSION..."
  go_package=go${GO_VERSION}.linux-amd64.tar.gz
  cd $RELEASE_REPO_ROOT/tool
  wget https://dl.google.com/go/$go_package
  tar xzvf $go_package -C $RELEASE_REPO_ROOT/tool
fi

GOROOT=$RELEASE_REPO_ROOT/tool/go

export PATH=$GOROOT/bin:$PATH
echo "PATH=$PATH"

rocksdb_dir=$RELEASE_REPO_ROOT/vendor/rocksdb
build_output_dir=$RELEASE_REPO_ROOT/code-build


if [[ ! -d "$rocksdb_dir/include/rocksdb" ]] || \
   [[ ! -f "$rocksdb_dir/lib/librocksdb.a" ]]; then
  cd $SOURCE_ROOT/third_party/rocksdb;make static_lib;\
    make INSTALL_PATH=$rocksdb_dir install 
fi

unset GOPATH

if [[ ! -d "$build_output_dir" ]]; then
  mkdir $build_output_dir
fi
 
export CGO_CFLAGS="-I$rocksdb_dir/include"
# export CGO_LDFLAGS="$rocksdb_dir/lib/librocksdb.a -lstdc++ -lm -lrt -lpthread -ldl"
export CGO_LDFLAGS="-L$rocksdb_dir/lib -L/usr/local/lib -lrocksdb -lstdc++ -lm -lrt -lpthread -ldl"

juno_version_info="-X juno/pkg/version.BuildTime=$build_time -X juno/pkg/version.Revision=$code_revision -X juno/pkg/version.BuildId=$JUNO_BUILD_NUMBER"

juno_executables="\
	github.com/paypal/junodb/cmd/proxy \
	github.com/paypal/junodb/cmd/storageserv \
	github.com/paypal/junodb/cmd/storageserv/storage/db/dbcopy \
	github.com/paypal/junodb/cmd/tools/junocli \
	github.com/paypal/junodb/cmd/clustermgr \
	github.com/paypal/junodb/cmd/dbscanserv \
	github.com/paypal/junodb/cmd/dbscanserv/junoctl \
        github.com/paypal/junodb/test/drv/junoload \
        github.com/paypal/junodb/cmd/tools/junostats\
        github.com/paypal/junodb/cmd/tools/junocfg\
	"
# DO NOT include junoload in any package

env GOBIN=$build_output_dir $RELEASE_REPO_ROOT/tool/go/bin/go install $build_tag --ldflags "-linkmode external -extldflags -static $juno_version_info" $juno_executables
cd $SOURCE_ROOT/cmd/etcdsvr; cp etcdctl etcdsvr.py etcdsvr_exe tool.py join.sh status.sh $build_output_dir;
cd $SOURCE_ROOT/cmd/clustermgr; cp store.sh swaphost.sh redist.sh $build_output_dir; 
