#!/bin/bash
#

# Distro Functions
# ================

# Determine OS Vendor, Release and Update

#
# NOTE : For portability, you almost certainly do not want to use
# these variables directly!  The "is_*" functions defined below this
# bundle up compatible platforms under larger umbrellas that we have
# determinted are compatible enough (e.g. is_ubuntu covers Ubuntu &
# Debian, is_fedora covers RPM-based distros).  Higher-level functions
# such as "install_package" further abstract things in better ways.
#
# ``os_VENDOR`` - vendor name: ``Ubuntu``, ``Fedora``, etc
# ``os_RELEASE`` - major release: ``16.04`` (Ubuntu), ``23`` (Fedora)
# ``os_PACKAGE`` - package type: ``deb`` or ``rpm``
# ``os_CODENAME`` - vendor's codename for release: ``xenial``

declare -g os_VENDOR os_RELEASE os_PACKAGE os_CODENAME

# Make a *best effort* attempt to install lsb_release packages for the
# user if not available.  Note can't use generic install_package*
# because they depend on this!
function _ensure_lsb_release {
    if [[ -x $(command -v lsb_release 2>/dev/null) ]]; then
        return
    fi

    if [[ -x $(command -v apt-get 2>/dev/null) ]]; then
        sudo apt-get install -y lsb-release
    elif [[ -x $(command -v zypper 2>/dev/null) ]]; then
        # XXX: old code paths seem to have assumed SUSE platforms also
        # had "yum".  Keep this ordered above yum so we don't try to
        # install the rh package.  suse calls it just "lsb"
        sudo zypper -n install lsb
    elif [[ -x $(command -v dnf 2>/dev/null) ]]; then
        sudo dnf install -y redhat-lsb-core
    elif [[ -x $(command -v yum 2>/dev/null) ]]; then
        # all rh patforms (fedora, centos, rhel) have this pkg
        sudo yum install -y redhat-lsb-core
    else
        die $LINENO "Unable to find or auto-install lsb_release"
    fi
}

# GetOSVersion
#  Set the following variables:
#  - os_RELEASE
#  - os_CODENAME
#  - os_VENDOR
#  - os_PACKAGE
function GetOSVersion {
    # We only support distros that provide a sane lsb_release
    _ensure_lsb_release

    os_RELEASE=$(lsb_release -r -s)
    os_CODENAME=$(lsb_release -c -s)
    os_VENDOR=$(lsb_release -i -s)

    if [[ $os_VENDOR =~ (Debian|Ubuntu|LinuxMint) ]]; then
        os_PACKAGE="deb"
    else
        os_PACKAGE="rpm"
    fi

    typeset -xr os_VENDOR
    typeset -xr os_RELEASE
    typeset -xr os_PACKAGE
    typeset -xr os_CODENAME
}

# Translate the OS version values into common nomenclature
# Sets global ``DISTRO`` from the ``os_*`` values
declare -g DISTRO

function GetDistro {
    GetOSVersion
    if [[ "$os_VENDOR" =~ (Ubuntu) || "$os_VENDOR" =~ (Debian) || \
            "$os_VENDOR" =~ (LinuxMint) ]]; then
        # 'Everyone' refers to Ubuntu / Debian / Mint releases by
        # the code name adjective
        DISTRO=$os_CODENAME
    elif [[ "$os_VENDOR" =~ (Fedora) ]]; then
        # For Fedora, just use 'f' and the release
        DISTRO="f$os_RELEASE"
    elif [[ "$os_VENDOR" =~ (openSUSE) ]]; then
        DISTRO="opensuse-$os_RELEASE"
    elif [[ "$os_VENDOR" =~ (SUSE LINUX) ]]; then
        # just use major release
        DISTRO="sle${os_RELEASE%.*}"
    elif [[ "$os_VENDOR" =~ (Red.*Hat) || \
        "$os_VENDOR" =~ (CentOS) || \
        "$os_VENDOR" =~ (Scientific) || \
        "$os_VENDOR" =~ (OracleServer) || \
        "$os_VENDOR" =~ (Virtuozzo) ]]; then
        # Drop the . release as we assume it's compatible
        # XXX re-evaluate when we get RHEL10
        DISTRO="rhel${os_RELEASE::1}"
    elif [[ "$os_VENDOR" =~ (XenServer) ]]; then
        DISTRO="xs${os_RELEASE%.*}"
    elif [[ "$os_VENDOR" =~ (kvmibm) ]]; then
        DISTRO="${os_VENDOR}${os_RELEASE::1}"
    else
        # We can't make a good choice here.  Setting a sensible DISTRO
        # is part of the problem, but not the major issue -- we really
        # only use DISTRO in the code as a fine-filter.
        #
        # The bigger problem is categorising the system into one of
        # our two big categories as Ubuntu/Debian-ish or
        # Fedora/CentOS-ish.
        #
        # The setting of os_PACKAGE above is only set to "deb" based
        # on a hard-coded list of vendor names ... thus we will
        # default to thinking unknown distros are RPM based
        # (ie. is_ubuntu does not match).  But the platform will then
        # also not match in is_fedora, because that also has a list of
        # names.
        #
        # So, if you are reading this, getting your distro supported
        # is really about making sure it matches correctly in these
        # functions.  Then you can choose a sensible way to construct
        # DISTRO based on your distros release approach.
        die $LINENO "Unable to determine DISTRO, can not continue."
    fi
    typeset -xr DISTRO
}

# Utility function for checking machine architecture
# is_arch arch-type
function is_arch {
    [[ "$(uname -m)" == "$1" ]]
}

# Determine if current distribution is an Oracle distribution
# is_oraclelinux
function is_oraclelinux {
    if [[ -z "$os_VENDOR" ]]; then
        GetOSVersion
    fi

    [ "$os_VENDOR" = "OracleServer" ]
}


# Determine if current distribution is a Fedora-based distribution
# (Fedora, RHEL, CentOS, etc).
# is_fedora
function is_fedora {
    if [[ -z "$os_VENDOR" ]]; then
        GetOSVersion
    fi

    [ "$os_VENDOR" = "Fedora" ] || [ "$os_VENDOR" = "Red Hat" ] || \
        [ "$os_VENDOR" = "RedHatEnterpriseServer" ] || \
        [ "$os_VENDOR" = "CentOS" ] || [ "$os_VENDOR" = "OracleServer" ] || \
        [ "$os_VENDOR" = "Virtuozzo" ] || [ "$os_VENDOR" = "kvmibm" ]
}


# Determine if current distribution is a SUSE-based distribution
# (openSUSE, SLE).
# is_suse
function is_suse {
    if [[ -z "$os_VENDOR" ]]; then
        GetOSVersion
    fi

    [[ "$os_VENDOR" =~ (openSUSE) || "$os_VENDOR" == "SUSE LINUX" ]]
}


# Determine if current distribution is an Ubuntu-based distribution
# It will also detect non-Ubuntu but Debian-based distros
# is_ubuntu
function is_ubuntu {
    if [[ -z "$os_PACKAGE" ]]; then
        GetOSVersion
    fi
    [ "$os_PACKAGE" = "deb" ]
}
