#!/bin/bash

# This scripts adds a new user to the MongoDB users collection inside the 
# running container without restarting the docker services. It automatically 
# generates the user id (uid) and the Auth Token which grants full access to the API.

ENV_PATH="/opt/micro-services.d/quotes-api/.environs"

if [ -f "$ENV_PATH" ]; then 
    source "$ENV_PATH"
else
    echo "Error: .environs file not found"
    exit 1
fi

# Check the an email is provided when called
if [ -z "$1" ]; then
    echo "Usage: ./add-user.sh <email>"
    exit 1
fi

EMAIL=$1

# SECURITY (Audit C-8): $EMAIL is interpolated into JavaScript executed by
# `mongo --eval` with ROOT credentials. Without validation, an argument like
#     x'}); db.users.drop(); //
# executes arbitrary database code. FAIL CLOSED: reject anything that is not
# a plausible email address BEFORE it can reach the eval string. The allowed
# character class contains no quotes, braces, backslashes, or whitespace, so
# a value that passes this gate cannot break out of the '...' JS literal.
if ! [[ "$EMAIL" =~ ^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$ ]]; then
    echo "Error: '$EMAIL' is not a valid email address. Aborting." >&2
    exit 1
fi
CONTAINER_NAME="quotes-database"
DB_NAME="qdb"

# First check to see if the user already exists. We dont want duplicates.
EXISTING_COUNT=$(docker exec "$CONTAINER_NAME" mongo "$DB_NAME" \
       	-u "$MONGO_INITDB_ROOT_USERNAME" \
	-p "$MONGO_INITDB_ROOT_PASSWORD" \
	--authenticationDatabase admin \
	--quiet \
	--eval "db.users.countDocuments({ email: '$EMAIL' })"
	)

# Clean up any carriage returns hidden in the output
EXISTING_COUNT=$(echo $EXISTING_COUNT | tr -dc '0-9')

if [ "$EXISTING_COUNT" -gt 0 ]; then 
    echo "this email address already exists in the database."
    echo "use list_users to locate them."
    exit 1
fi

# Generate UID
QUID=$(openssl rand -hex 16)

# Generate Auth Token
TOKEN=$(openssl rand -base64 32)

echo "Generating user for:   $EMAIL"
echo "Generating user id:    $QUID"
echo "Generating user token: $TOKEN"

echo "adding $EMAIL to MongoDB collection"

docker exec $CONTAINER_NAME mongo \
	"$DB_NAME" \
       	-u "$MONGO_INITDB_ROOT_USERNAME" \
	-p "$MONGO_INITDB_ROOT_PASSWORD" \
	--authenticationDatabase admin \
	--eval "db.users.insertOne({
	uid: '$QUID',
	email: '$EMAIL',
	authorization: '$TOKEN'
})"

echo "User authorized and added to the database"
