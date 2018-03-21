rm -R $HOME/assets/tst$2
mkdir -p  $HOME/assets/tst$2/wwwroot
mkdir -p  $HOME/assets/tst$2/log
cp -a /root/assets/trunk/wwwroot/* $HOME/assets/tst$2/wwwroot
chmod -R 777 $HOME/assets/tst$2/log

docker run --rm -p 25$21:25111 -p 25$22:25112 \
-v $HOME/assets/tst$2/log:/log \
-v $HOME/assets/trunk/apex5:/apex5 \
-v $HOME/assets/tst$2/wwwroot:/wwwroot \
-v $HOME/assets/tst$2/wwwroot/rolf:/ext/tst$2 \
--name=ipls_tst$2 iplsgo \
"-dsn=iplsql_reader/1@(DESCRIPTION=(ADDRESS = (PROTOCOL = TCP)(HOST = DP-AS-N3)(PORT = 1521))(CONNECT_DATA =(SERVICE_NAME = TST$1)))" \
"-conf=COMMON" \
"-conf_tm=10000" \
"-host=DP-ASW3" \
"-cs=(DESCRIPTION=(ADDRESS = (PROTOCOL = TCP)(HOST = DP-AS-N3)(PORT = 1521))(CONNECT_DATA =(SERVICE_NAME = TST$2)))"


#-v $HOME/assets/TST14/ROLF:/root/wwwroot/tst14
