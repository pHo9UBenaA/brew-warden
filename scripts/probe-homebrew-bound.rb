# Developer-only same-process lock and execution acceptance. The driver confines
# this process to a disposable prefix and freezes authenticated inputs.
require "formula_installer"
require "cmd/upgrade"
require "cmd/install"
require "timeout"

root = Pathname(ARGV.fetch(0))
operation = ARGV.fetch(1)
raise "unsupported operation" unless %w[install upgrade].include?(operation)
raise "VM required for upgrade" if operation == "upgrade" && !Utils.safe_popen_read("/usr/sbin/sysctl", "-n", "hw.model").start_with?("VirtualMac")
formulae = %w[jq oniguruma].map { |name| Formulary.factory("homebrew/core/#{name}") }
raise "unexpected existing locks" unless FormulaInstaller.locked.empty?
begin
  formulae.sort_by(&:name).each do |formula|
    formula.lock
    FormulaInstaller.locked << formula
  end
  # A distinct native Homebrew process must not acquire our target lock.
  code = 'require "lock_file"; FormulaLock.new("jq").lock; puts "UNEXPECTED_LOCK"'
  pid = Process.spawn(ENV.to_h, HOMEBREW_BREW_FILE.to_s, "ruby", "-e", code,
                      out: (root/"concurrent-lock.stdout").to_s,
                      err: (root/"concurrent-lock.stderr").to_s)
  _, status = Timeout.timeout(30) { Process.wait2(pid) }
  raise "concurrent lock unexpectedly acquired" if status.success?
  raise "wrong concurrent failure" unless (root/"concurrent-lock.stderr").read.include?("already locked")
  ARGV.replace([root.to_s, operation == "upgrade" ? "upgrade-before" : "before", "jq"])
  load root/"install-probe.rb"
  args = ["--formula", "--force-bottle", "homebrew/core/jq"]
  command = if operation == "install"
    Homebrew::Cmd::InstallCmd.new(args)
  else
    Homebrew::Cmd::UpgradeCmd.new(["--no-ask", *args])
  end
  command.run
  raise "native installation failed" if Homebrew.failed?
  raise "native installer released session locks" unless FormulaInstaller.locked == formulae.sort_by(&:name)
  ARGV.replace([root.to_s, operation == "upgrade" ? "upgrade-after" : "after", "jq"])
  load root/"install-probe.rb"
  puts "Verified same-process native execution under retained formula locks."
ensure
  FormulaInstaller.locked.reverse_each(&:unlock)
  FormulaInstaller.locked.clear
end
