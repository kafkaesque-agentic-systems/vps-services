#!/bin/bash
 

ENV_PATH="/opt/micro-services.d/quotes-api/.environs"

if [ -f "$ENV_PATH" ]; then 
    source "$ENV_PATH"
else
    echo "Error: .environs file not found"
    exit 1
fi


DB_NAME="qdb"
CONTAINER_NAME="quotes-database"

echo "USERS:"

docker exec $CONTAINER_NAME mongo \
	"$DB_NAME" \
       	-u "$MONGO_INITDB_ROOT_USERNAME" \
	-p "$MONGO_INITDB_ROOT_PASSWORD" \
	--authenticationDatabase admin \
	--quiet \
	--eval "db.users.find({}, {email: 1, uid: 1, authorization: 1, _id: 0}).pretty()"


