#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

COPY_DST_DIR="/host/opt/cni/bin"

CNI_BIN_DIR="/usr/plugins/cni"

echo "CNI_BIN_DIR=${CNI_BIN_DIR}"

[ -d "${CNI_BIN_DIR}" ] || { echo "error, failed to find ${CNI_BIN_DIR}" ; exit 1 ; }

mkdir -p ${COPY_DST_DIR} || true

for plugin in "${CNI_BIN_DIR}"/*; do
    ITEM=${plugin##*/}
    rm -f ${COPY_DST_DIR}/${ITEM}.old || true
    ( [ -f "${COPY_DST_DIR}/${ITEM}" ] && mv ${COPY_DST_DIR}/${ITEM} ${COPY_DST_DIR}/${ITEM}.old ) || true
    cp ${plugin} ${COPY_DST_DIR}
    rm -f ${COPY_DST_DIR}/${ITEM}.old &>/dev/null  || true
done

echo Done.
