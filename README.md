# Racelogger

This document describes how to use the racelogger. The developer documentation can be found [here](./README-dev.md).

The racelogger provides the following commands.

```console
racelogger.exe
```

```
Racelogger for the iRacelog project

Usage:
  racelogger [command]

Available Commands:
  check       check if racelogger is compatible with the backend server
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  import      import race from previous logged grpc messages file
  ping        check connection to backend server
  record      record an iRacing event
  status      check iracing status

Flags:
      --addr string         Address of the gRPC server
      --config string       config file (default is racelogger.yml)
  -h, --help                help for racelogger
      --insecure            allow insecure (non-tls) gRPC connections (used for development only)
      --log-file string     if present logs are written to this file, otherwise to stdout
      --log-format string   controls the log output format (json, text) (default "text")
      --log-level string    controls the log level (debug, info, warn, error, fatal) (default "info")
  -v, --version             version for racelogger

Use "racelogger [command] --help" for more information about a command.
```

Along with the executable comes a configuration file

```
# This will be the configuration file for the release process.
# It can be used a template

# Enter the address of the gRPC server
addr: grpc.iracing-tools.de:443
# Enter the dataprovider token
token:

log-level: info
log-format: json
# Enter the path to the log file. If not set, logs will be written to stdout
# Existing files will be replaced
#log-file: racelogger.log
```

| Key        | Value     | Info                                                  |
| ---------- | --------- | ----------------------------------------------------- |
| addr       | host:port | This is the address of the backend server             |
| token      |           | A secret credential to identify valid racelogger user |
| log-level  | info      | The level used for logging                            |
| log-format | json      | Logs are written in JSON format. May also use `text`  |
| log-file   |           | if present logs are written to this file              |

## Check

Enter the address of the backend server into the `racelogger.yml` file and perform a version check.

```console
racelogger.exe check
```

```
Racelogger version  : v0.11.1
Server version      : v0.14.2
Minimum racelogger  : v0.11.0
Compatible          : true
```

## Record

Here is an example how to record a race. Ensure the iRacing simulation is running and connected to a race.
Let's assume we are connected to a session of the Sebring 12h special event.

```console
racelogger.exe record -n "Sebring 12h" -d "Split #2"
```

This will start the recording and send data to the backend server every second. Once the race has finished the programm will stop.

_Tip:_ Use double quotes (") around values containing blanks and/or other special characters.

**Notes**:

-   Make sure you have set MaxCars to 63 in iRacing. This setting defines the amount of cars for which the iRacing server transfers data to the iRacing simulator.  
    In order to get a complete race overview we need the data for all cars. Note, this setting is just for the data transfer.  
    You find this setting in the iRacing simulator at Options -> Graphic  
    ![Max cars](docs/max-cars.png)

-   Make sure you have the highest available connection type setting active. You find this in your iRacing account page in the preferences section.  
     The setting DSL, Cable, Fiber, 1MBit/sec or faster seems to work best without losing any data.  
     ![](docs/account-settings.png)  
     When using a smaller value this may cause iRacing to send fewer car data at times which in turn causes the racelogger to assume that a car is offline and mark it as OUT during the period when no data is recieved for a particular car.

-   **Warning:** When recording you should not use the iRacing replay function. Some telemetry values will be invalidated when the replay mode is active. In such cases the racelogger may produce invalid data.

### Log messages while recording

You may want to log the messages that are sent to server. This may be useful if the connection to the server is lost. You may import the logged messages later.

```console
racelogger.exe record -n "Sebring 12h" -d "Split #2" --msg-log-file grpc-data.bin
```

The recorded messages are stored in a binary format in the file `grpc-data.bin`.

## Ping

To test the connection to server you may use the ping command. This will send 10 pings to the server with an interval of 1 second between two pings.

```console
racelogger.exe ping -n 10 -d 1s
```

## Import

Let's assume the connection to the backend server was lost during recording. Luckily we enabled to message logging during recording via the `--msg-log-file grpc-data.bin` option (see above).
After the race has finished we want to import the data to the backend. Best practise is to replace the (partial) data on the server with the import file.

```console
racelogger.exe import --replace-data grpc-data.bin
```
