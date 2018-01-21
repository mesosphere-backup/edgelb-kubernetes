FROM scratch
COPY edgelb_controller /edgelb_controller
COPY edgelb_client /edgelb_client
WORKDIR /
CMD ["/edgelb_controller", "-internal=true"]
