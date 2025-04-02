## Logging

This document provides an overview of  the recommended way to develop and implement logging.
Assisted migration project uses [zap](https://github.com/uber-go/zap) for logging.

### Logging Conventions

### How to log

There are two main zap methods for writing logs: `zap.Infof` and `zap.Errorf`. 

All structured logging methods accept the log message as a string, along with any number of key/value pairs that you provide via a variadic `interface{}` argument.
As variadic arguments represent key value pairs, they should always be even in count with first element being key of type string and second value of any type matching that key.
