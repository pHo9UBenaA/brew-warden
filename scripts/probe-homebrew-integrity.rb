# Exercise the native checksum cache using real files, without downloading or
# installing a bottle. Reverification is not a guarantee about later consumption.
require "downloadable"
require "digest"
require "json"

file = Pathname.new(ARGV.fetch(0))/"checksum-fixture"
original = "verified artifact bytes\n"
file.binwrite(original)
checksum = Checksum.new(Digest::SHA256.hexdigest(original))
cache = Downloadable::VerificationCache.new
cache.verify(file, checksum)
cache.verify(file, checksum)
stat = file.stat
file.binwrite("x" * original.bytesize)
File.utime(stat.atime, stat.mtime, file)
begin
  cache.verify(file, checksum)
  raise "cached byte substitution was accepted"
rescue ChecksumMismatchError
  # Expected, even after restoring the original size and modification time.
end
begin
  cache.verify(file, nil)
  raise "missing checksum was accepted"
rescue ChecksumMissingError
  # Expected.
end
puts JSON.pretty_generate({
  "originalBytesVerified" => true,
  "unchangedCacheVerified" => true,
  "sameSizeRestoredMtimeSubstitutionRejected" => true,
  "missingChecksumRejected" => true,
  "executionBindingEstablished" => false,
})
