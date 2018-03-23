docker run --rm -p 25$21:25111 -p 25$22:25112 \
-e SRC_URL=http://rolf-storm-dev:8071/RSP/master.zip \
-e TST_NUM=$2 \
-e DSN="-dsn=iplsql_reader/1@(DESCRIPTION=(ADDRESS = (PROTOCOL = TCP)(HOST = DP-AS-N3)(PORT = 1521))(CONNECT_DATA =(SERVICE_NAME = TST$1)))" \
-e CONF="-conf=COMMON" \
-e CONF_TM="-conf_tm=10" \
-e HOST="-host=DP-ASW3" \
-e CS="-cs=(DESCRIPTION=(ADDRESS = (PROTOCOL = TCP)(HOST = DP-AS-N3)(PORT = 1521))(CONNECT_DATA =(SERVICE_NAME = TST$2)))" \
-v $HOME/assets/tst$2/log:/log \
-v $HOME/assets/trunk/apex5:/apex5 \
-v $HOME/assets/trunk/wwwroot:/wwwroot \
-v $HOME/assets/tst$2/rolf:/ext/tst$2 \
--name=ipls_tst$2 iplsgo 

