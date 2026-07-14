#!/bin/bash

ENV_PATH="/opt/micro-services.d/quotes-api/.environs"

if [ -f "$ENV_PATH" ]; then 
    source "$ENV_PATH"
else
    echo "Error: .environs file not found"
    exit 1
fi

# Check the an email is provided when called
if [ -z "$1" ]; then 
    echo "Usage: ./remove_user.sh <email>"
    exit 1
fi

EMAIL=$1
CONTAINER_NAME="quotes-database"
DB_NAME="qdb"

# SECURITY (Audit C-8): $EMAIL is interpolated into JS executed by
# `mongo --eval` with root credentials. FAIL CLOSED: reject anything that is
# not a plausible email before it can reach the eval string (the allowed
# character class cannot break out of the '...' JS literal).
if ! [[ "$EMAIL" =~ ^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$ ]]; then
    echo "Error: '$EMAIL' is not a valid email address. Aborting." >&2
    exit 1
fi

echo "Deleting user: $EMAIL..."


docker exec $CONTAINER_NAME mongo \
	"$DB_NAME" \
       	-u "$MONGO_INITDB_ROOT_USERNAME" \
	-p "$MONGO_INITDB_ROOT_PASSWORD" \
	--authenticationDatabase admin \
	--eval "db.users.deleteOne({email: '$EMAIL'})"

echo "Done."


