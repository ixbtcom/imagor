# ixbtcom/imagor - delta vs upstream

Base: upstream `v1.9.2`.

- fix(s3storage): `Put` uses `Blob.NewReadSeeker()` so the AWS SDK can seek the
  body - to compute the signed payload hash over plain HTTP, and to rewind on a
  retryable error over HTTPS (SeaweedFS S3 gateway behind OPNsense HAProxy).
  Prevents "request stream is not seekable" save failures for loader-sourced
  originals.
- fix(imagor): single-flight the original storage save by `storageKey`, so N
  concurrent variants of one new original save it once (kills the parallel-save
  herd and the delete-after-save-error race that keeps originals from caching).
- fix(vipsprocessor): enable strict source decoding for both thumbnail and full
  image loads. A prematurely closed HTTP body is rejected instead of being
  encoded and cached as a structurally valid but visually corrupted AVIF/WebP.
- fix(vipsprocessor): return HTTP 424 for strict source-decode failures while
  keeping deterministic libvips processing errors on HTTP 406. This lets the
  ingress retry only a failed source dependency without flooding the slow pool
  with invalid transforms.
- fix(vipsprocessor): preserve the underlying source-reader failure separately
  from libvips' error buffer. Some truncated streams otherwise surface only as
  `vips: empty error buffer`; they now keep the same retryable HTTP 424 contract,
  while an empty buffer without a source failure remains HTTP 406.
- feat(imagor): add `IMAGOR_TIMEOUT_STATUS_CODE` with backward-compatible
  default HTTP 408. A fast role behind nginx can return retryable HTTP 424 for
  its own deadline, because nginx does not intercept an upstream HTTP 408;
  final fallback roles can keep 408 visible to clients and monitoring.

- feat(imagor): `IMAGOR_CONTENT_ETAG` (default off) sets a strong
  `ETag: "<md5 of body>-<format>"` on fresh and result-storage responses alike,
  answers a matching `If-None-Match` (list, `W/`, `*`) with 304, ignores
  `If-Modified-Since` when `If-None-Match` is present and drops `Last-Modified`.
  Caches in front (nginx `proxy_cache_revalidate`) revalidate by content, and a
  regenerated image with other bytes gets a new ETag.
- feat(storage): `Stat.ContentMD5` lets a storage hand over a known md5.
  filestorage publishes a file atomically (temp file in the same dir, xattr
  `user.imagor.md5 = <md5>:<size>`, then rename, or link for SaveErrIfExists),
  trusts the attribute only when the size matches and backfills it on the first
  read of an older file; s3storage takes the md5 from a single-part S3 ETag.

All listed changes are candidates for an upstream PR (kept out of the PR branch; fork-only doc).
