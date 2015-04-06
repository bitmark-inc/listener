# listener - A TLS listener that can be stopped properly.

This implements a single TLS listening channel over an underlying TCP
connection.  Multiple clients can connect.  A stop signal allows the
server to disconnect the clients in an orderly way.
