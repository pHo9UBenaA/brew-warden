require_relative "payload"
require "lock_file"
root = Pathname(ARGV.shift).realpath
names = ARGV.sort
raise "invalid reconciliation targets" unless !names.empty? && names.length <= 128 && names.uniq == names && names.all? { |name| name.match?(/\A[a-z0-9][a-z0-9+_.@-]{0,127}\z/) }
raise "unsupported reconciliation prefix" unless HOMEBREW_PREFIX.to_s == "/opt/homebrew"
locks = []
begin
  names.each { |name| lock = FormulaLock.new(name); lock.lock; locks << lock }
  state = names.map do |name|
    rack = HOMEBREW_CELLAR/name
    payload = if rack.symlink?
      { symlink: rack.readlink.to_s }
    elsif rack.directory?
      BrewWardenPayload.inventory(rack)
    elsif !rack.exist?
      { absent: true }
    else
      raise "unsupported candidate rack type"
    end
    opt = HOMEBREW_PREFIX/"opt"/name
    linked = HOMEBREW_LINKED_KEGS/name
    { name:, payload:, opt: opt.symlink? ? opt.readlink.to_s : nil, linked: linked.symlink? ? linked.readlink.to_s : nil }
  end
  puts JSON.generate({ schema: 1, afterState: BrewWardenPayload.store_snapshot(root, state) })
ensure
  locks.reverse_each(&:unlock)
end
