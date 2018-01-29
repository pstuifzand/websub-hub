FROM ubuntu
ADD ./hubserver /usr/local/bin
EXPOSE 80
ENTRYPOINT ["/usr/local/bin/hubserver"]
