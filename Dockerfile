FROM alpine

COPY ./expose /usr/local/bin/expose

ENTRYPOINT ["/usr/local/bin/expose"]