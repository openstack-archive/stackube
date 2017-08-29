#!/bin/bash
# Copyright (c) 2017 OpenStack Foundation.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# !! source _before_ any services that use ``SERVICE_HOST``
#
# Dependencies:
#
# - ``DEST``, ``DATA_DIR`` must be defined
# - ``HOST_IP``, ``SERVICE_HOST``
# - ``KEYSTONE_TOKEN_FORMAT`` must be defined

# Entry points:
#
# - configure_CA
# - init_CA
# - cleanup_CA
# - make_root_CA
# - make_int_CA
# - make_cert ca-dir cert-name "common-name" ["alt-name" ...]



# Defaults
# --------

# TODO: support more distributions
function is_fedora {
    # Always true
    return 0
}

# Check if this is a valid ipv4 address string
function is_ipv4_address {
    local address=$1
    local regex='([0-9]{1,3}.){3}[0-9]{1,3}'
    # TODO(clarkb) make this more robust
    if [[ "$address" =~ $regex ]] ; then
        return 0
    else
        return 1
    fi
}


SSL_BUNDLE_FILE="$DATA_DIR/ca-bundle.pem"
TLS_IP=${TLS_IP:-$SERVICE_IP}

STACKUBE_HOSTNAME=$(hostname -f)
STACKUBE_CERT_NAME=stackube-cert
STACKUBE_CERT=$DATA_DIR/$STACKUBE_CERT_NAME.pem

# CA configuration
ROOT_CA_DIR=${ROOT_CA_DIR:-$DATA_DIR/CA/root-ca}
INT_CA_DIR=${INT_CA_DIR:-$DATA_DIR/CA/int-ca}

ORG_NAME="OpenStack"
ORG_UNIT_NAME="Stackube"


# CA Functions
# ============

# There may be more than one, get specific
OPENSSL=${OPENSSL:-/usr/bin/openssl}

# Do primary CA configuration
function configure_CA {
    # build common config file

    # Verify ``TLS_IP`` is good
    if [[ -n "$HOST_IP" && "$HOST_IP" != "$TLS_IP" ]]; then
        # auto-discover has changed the IP
        TLS_IP=$HOST_IP
    fi
}

# Creates a new CA directory structure
# create_CA_base ca-dir
function create_CA_base {
    local ca_dir=$1

    if [[ -d $ca_dir ]]; then
        # Bail out it exists
        return 0
    fi

    local i
    for i in certs crl newcerts private; do
        mkdir -p $ca_dir/$i
    done
    chmod 710 $ca_dir/private
    echo "01" >$ca_dir/serial
    cp /dev/null $ca_dir/index.txt
}

# Create a new CA configuration file
# create_CA_config ca-dir common-name
function create_CA_config {
    local ca_dir=$1
    local common_name=$2

    echo "
[ ca ]
default_ca = CA_default

[ CA_default ]
dir                     = $ca_dir
policy                  = policy_match
database                = \$dir/index.txt
serial                  = \$dir/serial
certs                   = \$dir/certs
crl_dir                 = \$dir/crl
new_certs_dir           = \$dir/newcerts
certificate             = \$dir/cacert.pem
private_key             = \$dir/private/cacert.key
RANDFILE                = \$dir/private/.rand
default_md              = sha256

[ req ]
default_bits            = 2048
default_md              = sha256

prompt                  = no
distinguished_name      = ca_distinguished_name

x509_extensions         = ca_extensions

[ ca_distinguished_name ]
organizationName        = $ORG_NAME
organizationalUnitName  = $ORG_UNIT_NAME Certificate Authority
commonName              = $common_name

[ policy_match ]
countryName             = optional
stateOrProvinceName     = optional
organizationName        = match
organizationalUnitName  = optional
commonName              = supplied

[ ca_extensions ]
basicConstraints        = critical,CA:true
subjectKeyIdentifier    = hash
authorityKeyIdentifier  = keyid:always, issuer
keyUsage                = cRLSign, keyCertSign

" >$ca_dir/ca.conf
}

# Create a new signing configuration file
# create_signing_config ca-dir
function create_signing_config {
    local ca_dir=$1

    echo "
[ ca ]
default_ca = CA_default

[ CA_default ]
dir                     = $ca_dir
policy                  = policy_match
database                = \$dir/index.txt
serial                  = \$dir/serial
certs                   = \$dir/certs
crl_dir                 = \$dir/crl
new_certs_dir           = \$dir/newcerts
certificate             = \$dir/cacert.pem
private_key             = \$dir/private/cacert.key
RANDFILE                = \$dir/private/.rand
default_md              = default

[ req ]
default_bits            = 1024
default_md              = sha1

prompt                  = no
distinguished_name      = req_distinguished_name

x509_extensions         = req_extensions

[ req_distinguished_name ]
organizationName        = $ORG_NAME
organizationalUnitName  = $ORG_UNIT_NAME Server Farm

[ policy_match ]
countryName             = optional
stateOrProvinceName     = optional
organizationName        = match
organizationalUnitName  = optional
commonName              = supplied

[ req_extensions ]
basicConstraints        = CA:false
subjectKeyIdentifier    = hash
authorityKeyIdentifier  = keyid:always, issuer
keyUsage                = digitalSignature, keyEncipherment, keyAgreement
extendedKeyUsage        = serverAuth, clientAuth
subjectAltName          = \$ENV::SUBJECT_ALT_NAME

" >$ca_dir/signing.conf
}

# Create root and intermediate CAs
# init_CA
function init_CA {
    # Ensure CAs are built
    make_root_CA $ROOT_CA_DIR
    make_int_CA $INT_CA_DIR $ROOT_CA_DIR

    # Create the CA bundle
    cat $ROOT_CA_DIR/cacert.pem $INT_CA_DIR/cacert.pem >>$INT_CA_DIR/ca-chain.pem
    cat $INT_CA_DIR/ca-chain.pem >> $SSL_BUNDLE_FILE

    if is_fedora; then
        sudo cp $INT_CA_DIR/ca-chain.pem /usr/share/pki/ca-trust-source/anchors/stackube-chain.pem
        sudo update-ca-trust
    elif is_suse; then
        sudo cp $INT_CA_DIR/ca-chain.pem /usr/share/pki/trust/anchors/stackube-chain.pem
        sudo update-ca-certificates
    elif is_ubuntu; then
        sudo cp $INT_CA_DIR/ca-chain.pem /usr/local/share/ca-certificates/stackube-int.crt
        sudo cp $ROOT_CA_DIR/cacert.pem /usr/local/share/ca-certificates/stackube-root.crt
        sudo update-ca-certificates
    fi
}

# Create an initial server cert
# init_cert
function init_cert {
    if [[ ! -r $STACKUBE_CERT ]]; then
        if [[ -n "$TLS_IP" ]]; then
            # Lie to let incomplete match routines work
            TLS_IP="DNS:$TLS_IP,IP:$TLS_IP"
        fi
        make_cert $INT_CA_DIR $STACKUBE_CERT_NAME $STACKUBE_HOSTNAME "$TLS_IP"

        # Create a cert bundle
        cat $INT_CA_DIR/private/$STACKUBE_CERT_NAME.key $INT_CA_DIR/$STACKUBE_CERT_NAME.crt $INT_CA_DIR/cacert.pem >$STACKUBE_CERT
    fi
}

# make_cert creates and signs a new certificate with the given commonName and CA
# make_cert ca-dir cert-name "common-name" ["alt-name" ...]
function make_cert {
    local ca_dir=$1
    local cert_name=$2
    local common_name=$3
    local alt_names=$4

    if [ "$common_name" != "$SERVICE_HOST" ]; then
        if [[ -z "$alt_names" ]]; then
            alt_names="DNS:$SERVICE_HOST"
        else
            alt_names="$alt_names,DNS:$SERVICE_HOST"
        fi
        if is_ipv4_address "$SERVICE_HOST" ; then
            alt_names="$alt_names,IP:$SERVICE_HOST"
        fi
    fi

    # Only generate the certificate if it doesn't exist yet on the disk
    if [ ! -r "$ca_dir/$cert_name.crt" ]; then
        # Generate a signing request
        $OPENSSL req \
            -sha1 \
            -newkey rsa \
            -nodes \
            -keyout $ca_dir/private/$cert_name.key \
            -out $ca_dir/$cert_name.csr \
            -subj "/O=${ORG_NAME}/OU=${ORG_UNIT_NAME} Servers/CN=${common_name}"

        if [[ -z "$alt_names" ]]; then
            alt_names="DNS:${common_name}"
        else
            alt_names="DNS:${common_name},${alt_names}"
        fi

        # Sign the request valid for 1 year
        SUBJECT_ALT_NAME="$alt_names" \
        $OPENSSL ca -config $ca_dir/signing.conf \
            -extensions req_extensions \
            -days 3650 \
            -notext \
            -in $ca_dir/$cert_name.csr \
            -out $ca_dir/$cert_name.crt \
            -subj "/O=${ORG_NAME}/OU=${ORG_UNIT_NAME} Servers/CN=${common_name}" \
            -batch
    fi
}

# Make an intermediate CA to sign everything else
# make_int_CA ca-dir signing-ca-dir
function make_int_CA {
    local ca_dir=$1
    local signing_ca_dir=$2

    # Create the root CA
    create_CA_base $ca_dir
    create_CA_config $ca_dir 'Intermediate CA'
    create_signing_config $ca_dir

    if [ ! -r "$ca_dir/cacert.pem" ]; then
        # Create a signing certificate request
        $OPENSSL req -config $ca_dir/ca.conf \
            -sha1 \
            -newkey rsa \
            -nodes \
            -keyout $ca_dir/private/cacert.key \
            -out $ca_dir/cacert.csr \
            -outform PEM

        # Sign the intermediate request valid for 1 year
        $OPENSSL ca -config $signing_ca_dir/ca.conf \
            -extensions ca_extensions \
            -days 3650 \
            -notext \
            -in $ca_dir/cacert.csr \
            -out $ca_dir/cacert.pem \
            -batch
    fi
}

# Make a root CA to sign other CAs
# make_root_CA ca-dir
function make_root_CA {
    local ca_dir=$1

    # Create the root CA
    create_CA_base $ca_dir
    create_CA_config $ca_dir 'Root CA'

    if [ ! -r "$ca_dir/cacert.pem" ]; then
        # Create a self-signed certificate valid for 5 years
        $OPENSSL req -config $ca_dir/ca.conf \
            -x509 \
            -nodes \
            -newkey rsa \
            -days 21360 \
            -keyout $ca_dir/private/cacert.key \
            -out $ca_dir/cacert.pem \
            -outform PEM
    fi
}




# Cleanup Functions
# =================

# Clean up the CA files
# cleanup_CA
function cleanup_CA {
    if is_fedora; then
        sudo rm -f /usr/share/pki/ca-trust-source/anchors/stackube-chain.pem
        sudo update-ca-trust
    elif is_ubuntu; then
        sudo rm -f /usr/local/share/ca-certificates/stackube-int.crt
        sudo rm -f /usr/local/share/ca-certificates/stackube-root.crt
        sudo update-ca-certificates
    fi

    rm -rf "$INT_CA_DIR" "$ROOT_CA_DIR" "$STACKUBE_CERT"
}

