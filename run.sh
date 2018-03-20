rm -R $HOME/assets/$1
mkdir -p  $HOME/assets/$1
cp -a /root/assets/mastercopy/* $HOME/assets/$1

docker run --rm -p 25111:25111 -p 25112:25112 -v $HOME/assets/tst14:/root/wwwroot -v $HOME/assets/$1/rolf:/root/ext/$1 --name=ipls_$1 iplsgo \
"-dsn=iplsql_reader/1@(DESCRIPTION=(ADDRESS = (PROTOCOL = TCP)(HOST = DP-AS-N3)(PORT = 1521))(CONNECT_DATA =(SERVICE_NAME = $1)))" \
"-conf=COMMON" \
"-conf_tm=10000" \
"-host=DP-ASW3" \
"-cs=(DESCRIPTION=(ADDRESS = (PROTOCOL = TCP)(HOST = DP-AS-N3)(PORT = 1521))(CONNECT_DATA =(SERVICE_NAME = $1)))"


#-v $HOME/assets/TST14/ROLF:/root/wwwroot/tst14
