# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM golang:1.5.3

ENV DEBIAN_FRONTEND noninteractive

ENV ORACLE_INSTANTCLIENT_MAJOR 12.1
ENV ORACLE_INSTANTCLIENT_VERSION 12.1.0.2.0

ENV ORACLE /usr/local/oracle
ENV ORACLE_HOME $ORACLE/lib/oracle/$ORACLE_INSTANTCLIENT_MAJOR/client64
ENV LD_LIBRARY_PATH $ORACLE_HOME/lib
ENV C_INCLUDE_PATH $ORACLE/include/oracle/$ORACLE_INSTANTCLIENT_MAJOR/client64
ENV CGO_CFLAGS -I$C_INCLUDE_PATH
ENV CGO_LDFLAGS "-L$LD_LIBRARY_PATH -locci -lnnz12 -lclntsh"
ENV TNS_ADMIN $ORACLE_HOME

ENV GO15VENDOREXPERIMENT=1 

RUN apt-get update && apt-get install -y libaio1 \
        curl rpm2cpio cpio unzip \
    && mkdir -p $GOPATH/src/github.com/vsdutka/iplsgo \
    && cd $GOPATH/src/github.com/vsdutka \
    && wget --no-check-certificate -O master.zip "https://github.com/vsdutka/iplsgo/archive/master.zip" \ 
    && unzip master.zip \
    && mv iplsgo-master/* iplsgo \
    && cd iplsgo \
    && ls -F \
    && mkdir $ORACLE && TMP_DIR="$(mktemp -d)" && cd "$TMP_DIR" \
    && curl -L https://github.com/sergeymakinen/docker-oracle-instant-client/raw/assets/oracle-instantclient$ORACLE_INSTANTCLIENT_MAJOR-basic-$ORACLE_INSTANTCLIENT_VERSION-1.x86_64.rpm -o basic.rpm \
    && rpm2cpio basic.rpm | cpio -i -d -v && cp -r usr/* $ORACLE && rm -rf ./* \
    && curl -L https://github.com/sergeymakinen/docker-oracle-instant-client/raw/assets/oracle-instantclient$ORACLE_INSTANTCLIENT_MAJOR-devel-$ORACLE_INSTANTCLIENT_VERSION-1.x86_64.rpm -o devel.rpm \
    && rpm2cpio devel.rpm | cpio -i -d -v && cp -r usr/* $ORACLE \
    && echo "$ORACLE_HOME/lib" > /etc/ld.so.conf.d/oracle.conf && chmod o+r /etc/ld.so.conf.d/oracle.conf && ldconfig \
    && rm -rf /var/lib/apt/lists/* \
    && cd $GOPATH/src/github.com/vsdutka/iplsgo \
    && go install github.com/vsdutka/iplsgo \
    && rm -rf /var/lib/apt/lists/* && apt-get purge -y --auto-remove curl rpm2cpio cpio unzip \
    && mkdir /go/bin/log


ENTRYPOINT ["/go/bin/iplsgo"]
