FROM scratch
COPY edgelb_controller /edgelb_controller
WORKDIR /
CMD ["/edgelb_controller", "-internal=true"]
