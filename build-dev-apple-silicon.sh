#!/bin/bash

set -eu -o pipefail

# https://github.com/DataDog/datadog-agent/blob/main/docs/dev/agent_build.md
# https://github.com/DataDog/datadog-agent/blob/main/docs/dev/agent_dev_env.md

# override this as you wish: find these definitions in release.json
RELEASE_VERSION=release-a7

function sanity_checks() {
    # Brew env
    if brew ls | grep gettext >/dev/null; then
        echo "Won't proceed since we found a 'gettext' in 'brew ls'. This is known to cause"
        echo "issues in the build. Uninstall the brew gettext library by running:"
        echo ""
        echo "'brew rm --ignore-dependencies gettext'"
        exit 1
    fi

    # Ruby env
    if [[ "$(command -v ruby)" = "/usr/bin/ruby" ]]; then
        echo "Exiting early because you've got a system Ruby selected. First select a 2.7.x ruby using"
        echo "a ruby version manager and retry."
        exit 1
    fi
    # Old omnibus fork doesn't yet support Ruby 3.0+
    if ! ruby --version | grep -E -q '^ruby 2\.7.*$' >/dev/null; then
        echo "A ruby version other than 2.7 was detected. Switch first to a 2.7 Ruby version and retry."
        exit 1
    fi

    # Python env
    if ! command -v python3.9 >/dev/null; then
        echo "This script requires you have an available Python 3.9 in your PATH, but one couldn't"
        echo "be found. Exiting early."
        exit 1
    fi
}

function env_setup() {
    # required directories
    for builddir in /var/cache/omnibus /opt/datadog-agent; do
        if [ ! -d "${builddir}" ]; then
            echo "Missing required dir: ${builddir}. Will need to create this and chown it to "
            echo "${USER} using sudo, which may now prompt for sudo credentials."
            sudo mkdir -p "${builddir}"
            sudo chown "$(whoami)" "${builddir}"
        fi
    done

    # git: if we had a homebrew-installed git already installed, we just broke it by uninstalling
    #      gettext as required in the earlier step. So, we set up a shim /bin dir at the front of
    #      PATh which points back to /usr/bin/git
    path_shim="$(mktemp -d)"
    mkdir "${path_shim}/bin"
    ln -sf /usr/bin/git "${path_shim}/bin/git"
    export PATH="${path_shim}/bin:${PATH}"

    # python
    python_exe="$(command -v python3.9)"
    if [ ! -d venv ]; then
        mkdir venv
        "${python_exe}" -m venv venv
    fi
    source venv/bin/activate
    pip install -r requirements.txt --disable-pip-version-check

    # go
    command -v gimme || brew install gimme
    go_version=$(cat .go-version)
    eval "$(gimme "${go_version}")"
    inv check-go-version

    # We should only need this for dev/testing reasons, not packaged builds
    # invoke install-tools
}

sanity_checks
env_setup

# including --log-level=debug so we get full configure/make output
invoke \
    --echo \
    agent.omnibus-build \
    --skip-sign \
    --python-runtimes "3" \
    --major-version "7" \
    --release-version "${RELEASE_VERSION}" \
    --log-level=debug
