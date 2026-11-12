# Third-party notices

## Offline QR code generation

`github.com/skip2/go-qrcode` is pinned to `v0.0.0-20200617195104-da1b6568686e`
and included in the device binary under the MIT license (Copyright 2014 Tom
Harwood). The license is reproduced in `docs/LICENSE-qrcode.txt`.
Source: <https://github.com/skip2/go-qrcode>. This pure-Go encoder generates the
wireless transfer QR locally; it requires no external QR service or device runtime.

## In-memory S3 integration tests

`github.com/johannesboyne/gofakes3` is pinned to
`v0.0.0-20250402064820-d479899d8cbe` (MIT, Copyright 2016 Johannes Boyne).
Its license is reproduced in `docs/LICENSE-gofakes3.txt`.
Source: <https://github.com/johannesboyne/gofakes3/tree/d479899d8cbe>.
The `s3mem` backend and its transitive dependencies are used only by Go tests;
they are not linked into the device binary or bundled as server software.
The small test-only Range status correction follows upstream
<https://github.com/johannesboyne/gofakes3/pull/121>.

## Optional KUAL navigation patch tooling

`scripts/kual-navigation/build.py` downloads hash-pinned ASM 9.7.1 for host-side
bytecode editing only. ASM is BSD-3-Clause licensed:
<https://asm.ow2.io/license.html>. Its JAR stays in the ignored local build cache;
it is not installed on the Kindle or included in client packages.
KUAL belongs to its upstream authors; Booklet source reference:
<https://github.com/coplate/KUAL_Booklet>.
The patch requires the owner's exact original JAR; neither original nor patched
KUAL binaries are distributed by this repository.

## Mozilla CA certificate bundle

`internal/cloudhttp/cacert.pem` is Mozilla's public CA certificate list converted
and distributed by the curl project: <https://curl.se/docs/caextract.html>.
Source: <https://curl.se/ca/cacert.pem>. The bundle is covered by the Mozilla
Public License 2.0: <https://www.mozilla.org/MPL/2.0/>. Its generation timestamp
and source information are preserved in the file header. It is embedded so old
Kindle firmware does not need a current system trust store. TLS certificate and
hostname validation remain enabled. The license is reproduced in
`docs/LICENSE-mpl-2.0.txt`; the unmodified PEM source is included in this
repository at the path above: <https://github.com/mr-jimmy6/inkpika/blob/main/internal/cloudhttp/cacert.pem>.

## QingStor protocol reference

The native signature and URL encoding follow the QingStor Go SDK protocol:
<https://github.com/qingstor/qingstor-sdk-go/tree/master/request/signer/v2> and
<https://github.com/qingstor/qingstor-sdk-go/blob/master/utils/escape.go>.
SDK copyright: Yunify, Inc.; Apache License 2.0. No SDK dependency is bundled.
Signature test vectors are reproduced from the SDK's Apache-2.0 test suite;
the license is reproduced in `docs/LICENSE-apache-2.0.txt`.
The local tests adapt these vectors to this project's implementation.

## Go runtime

Release binaries include the Go 1.23.12 runtime and standard library under the
BSD 3-Clause license, reproduced in `docs/LICENSE-go.txt`:
<https://go.dev/LICENSE>.

## Alibaba Cloud OSS protocol reference

OSS V4 signing, virtual hosting and ListObjectsV2 follow the official OSS Go v2
SDK: <https://github.com/aliyun/alibabacloud-oss-go-sdk-v2>.
Signature fixtures use the expected output values published in its
`oss/signer/signer_test.go`. The SDK and these adapted fixtures are covered
by Apache License 2.0: <https://github.com/aliyun/alibabacloud-oss-go-sdk-v2/blob/master/LICENSE>,
reproduced in `docs/LICENSE-apache-2.0.txt`. No OSS SDK dependency is bundled.
