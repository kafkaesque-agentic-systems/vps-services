#!/bin/bash

ENV_PATH="/opt/micro-services.d/quotes-api/.environs"

if [ -f "$ENV_PATH" ]; then 
    source "$ENV_PATH"
else
    echo "Error: .environs file not found"
    exit 1
fi

# validate argument input
if [ -z "$1" ]; then 
    echo "Usage: ./find_user_by_post_id <object id>"
    exit 1
fi

OBJ_ID=$1
DB_NAME="qdb"
CONTAINER_NAME="quotes-database"

# SECURITY (Audit C-8): $OBJ_ID is interpolated into JS executed by
# `mongo --eval` with root credentials. FAIL CLOSED: a MongoDB ObjectId is
# exactly 24 hex characters — anything else is rejected before it can reach
# the eval string.
if ! [[ "$OBJ_ID" =~ ^[0-9a-fA-F]{24}$ ]]; then
    echo "Error: '$OBJ_ID' is not a valid 24-character hex ObjectId. Aborting." >&2
    exit 1
fi

echo "looking for quote by id: $OBJ_ID"


uid_result=$(docker exec $CONTAINER_NAME mongo \
	"$DB_NAME" \
       	-u "$MONGO_INITDB_ROOT_USERNAME" \
	-p "$MONGO_INITDB_ROOT_PASSWORD" \
	--authenticationDatabase admin \
	--quiet \
	--eval "var doc = db.qdata.findOne({ _id: ObjectId('$OBJ_ID') }); if (doc) { print(doc.uid); }")

echo "USER ID: $uid_result"

