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

Both are candidates for an upstream PR (kept out of the PR branch; fork-only doc).
