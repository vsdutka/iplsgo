docker build -t oracle-instantclient-12-1 -f Dockerfile.oracle-instantclient.12.1 .
docker build -t go153-oracle-instantclient-12-1 -f Dockerfile.base-builder .

docker build -t iplsgo-builder --no-cache -f Dockerfile.build .
docker run --rm iplsgo-builder | docker build -t iplsgo  -f Dockerfile.run -

docker run --rm -p 221$2:25111 -p 222$2:25112 \
-e SRC_URL=???? \
-e TST_NUM=$2 \
-e DSN="-dsn=iplsql_reader/1@(DESCRIPTION=(ADDRESS = (PROTOCOL = TCP)(HOST = ???)(PORT = 1521))(CONNECT_DATA =(SERVICE_NAME = ???)))" \
-e CONF="-conf=???" \
-e CONF_TM="-conf_tm=10" \
-e HOST="-host=????" \
-e CS="-cs=(DESCRIPTION=(ADDRESS = (PROTOCOL = TCP)(HOST = ????)(PORT = 1521))(CONNECT_DATA =(SERVICE_NAME = ???)))" \
-v $HOME/assets/tst$2/log:/log \
-v $HOME/assets/trunk/apex5:/apex5 \
-v $HOME/assets/trunk/wwwroot:/wwwroot \
-v $HOME/assets/tst$2/rolf:/ext/tst$2 \
--name=ipls_tst$2 iplsgo 