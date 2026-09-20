# Reuse Homebrew's environment/bootstrap, then select the installation prefix
# before loading its constants in a fresh Ruby process. Source remains private.
prefix = ARGV.shift
raise "unsupported installation prefix" unless prefix == "/opt/homebrew"
ENV["HOMEBREW_PREFIX"] = prefix
ENV["HOMEBREW_CELLAR"] = "#{prefix}/Cellar"
ENV["HOMEBREW_CASKROOM"] = "#{prefix}/Caskroom"
exec(*HOMEBREW_RUBY_EXEC_ARGS, "-I", $LOAD_PATH.join(File::PATH_SEPARATOR), "-rglobal", *ARGV)
