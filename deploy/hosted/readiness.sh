#!/usr/bin/env bash

serenity_wait_ready() {
    local ready_url=${1:?usage: serenity_wait_ready URL}
    curl --fail --silent --show-error \
        --header 'Cache-Control: no-cache' \
        --max-time 2 \
        --retry 5 \
        --retry-connrefused \
        --retry-delay 1 \
        --retry-max-time 15 \
        "$ready_url"
}

serenity_rollback_on_readiness_failure() {
    local active_binary=${1:?usage: serenity_rollback_on_readiness_failure ACTIVE PREVIOUS UNIT URL}
    local previous_binary=${2:?usage: serenity_rollback_on_readiness_failure ACTIVE PREVIOUS UNIT URL}
    local service=${3:?usage: serenity_rollback_on_readiness_failure ACTIVE PREVIOUS UNIT URL}
    local ready_url=${4:?usage: serenity_rollback_on_readiness_failure ACTIVE PREVIOUS UNIT URL}

    if serenity_wait_ready "$ready_url"; then
        return 0
    fi

    if [[ ! -x "$previous_binary" ]]; then
        echo "Readiness failed and no previous binary is available at $previous_binary" >&2
        return 1
    fi

    local previous_target
    previous_target=$(readlink -f -- "$previous_binary") || {
        echo "Readiness failed and the previous binary could not be resolved" >&2
        return 1
    }
    ln -sfn -- "$previous_target" "$active_binary"
    if systemctl restart "$service"; then
        echo "ROLLED BACK to $previous_target" >&2
    else
        echo "ROLLED BACK to $previous_target, but $service restart failed" >&2
    fi
    return 1
}
