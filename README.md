# Weblogic exporter - lite

## Features 

- Collects metrics for a single WebLogic server
- Checks admin server status and reports up/down state
- Exposes metrics via HTTP endpoint at 0.0.0.0:9255/metrics
     

## Usage
```bash
Usage: ./weblogic_check -admin-url <URL> -username <user> -password <pass> -server-name <name> [-port <listening-port>]
```

### Command-line Flags 

The exporter takes in several command-line flags: 

* admin-url:	URL of the WebLogic admin server (e.g., http://localhost:7001 )
* username:	Username for WebLogic admin server
* password:	Password for WebLogic admin server (This will be in clear text in the ps output.)
* server-name:	Name of the WebLogic server to monitor (e.g., AdminServer)
* port:	Port for the exporter (default: 9255)
