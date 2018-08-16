#!/bin/bash

echo -e "Content-Type: text/plain; charset=utf-8\n\nhello, cgi\n\nQUERY_STRING: ${QUERY_STRING}\nINPUT: $(cat /proc/self/fd/0)"
