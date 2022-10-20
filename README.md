# Database Interface Controllers
## Database Controller
Database controller is responsible to manage lifecycle of database objects.
Specifically, this controller monitors the lifecycle of the user-facing CRDs:

- DatabaseRequest - Represents a request to provision the Database
- DatabaseAccessRequest - Represents a request to access the Database

and generates the associated CRDs:

- Database - Represents a Database
- DatabaseAccess - Represents an access secret to the Database

# Database Sidecar Controller

Database provisioner sidecar is responsible to manage lifecycle of Database Interface objects and is
deployed as a sidecar to a provisioner. Specifically, the sidecar monitors the lifecycle of the CRDs generated by the Database Controller
and makes gRPC calls to the associated provisioner.