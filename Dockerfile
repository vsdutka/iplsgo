FROM alpine
WORKDIR iplsgo
ADD hello hello
ADD iplsgo iplsgo

ENV ORACLE_INSTANTCLIENT_MAJOR 12.2
ENV ORACLE_INSTANTCLIENT_VERSION 12.2.0.1.0

ENV ORACLE /usr/lib/oracle
ENV ORACLE_HOME $ORACLE/$ORACLE_INSTANTCLIENT_MAJOR/client64
#RUN echo "QQQQQQQQq - $ORACLE_HOME/lib"
#RUN export -p
RUN mkdir -p $ORACLE_HOME/lib
ADD lib $ORACLE_HOME/lib

#RUN touch /etc/ld.so.conf.d/ora-inst-cl-$ORACLE_INSTANTCLIENT_VERSION.conf
#RUN echo "/usr/lib/oracle/12.2/client64/lib" > /etc/ld.so.conf.d/ora-inst-cl-$ORACLE_INSTANTCLIENT_VERSION.conf

# обновляем кэша динамических библиотек 
# и делаем библиотеки видимыми для системы
#RUN sudo ldconfig

#RUN ls
#RUN ls iplsgo/

ENV PATH $PATH:$ORACLE_HOME/lib 
#ENTRYPOINT [".iplsgo", "-dsn=iplsql_reader/1@dp-se-tst1", "-conf=SE_unionASR.xcfg", "-conf_tm=1000000", "-host=DP-ASW3"] 

CMD export -p && ls -F && ./hello && ./iplsgo "-dsn=iplsql_reader/1@dp-se-tst1" "-conf=SE_unionASR.xcfg" "-conf_tm=1000000" "-host=DP-ASW3"

# Document that the service listens on port 8080.
EXPOSE 11119
