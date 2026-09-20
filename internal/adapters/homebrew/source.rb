# Recognize previously reviewed build methods with Ruby's maintained parser.
# This is deliberately narrow, not a general proof about arbitrary Ruby effects.
require "ripper"
require "json"
require "digest"

module BrewWardenSource
  REVIEWED_INSTALL = {
    "jq" => "1d35bb829c7e58ed32598212783ddb4c7d5975c4296a687d321dfa85f0666c25",
    "oniguruma" => "b6508e942446a95afbba964d6f83d5a1e4a7e2a2e38cb3abbed348c2e755d53d",
  }.freeze

  def self.normalize(node)
    return node unless node.is_a?(Array)
    # Strip only parser source coordinates, preserving token category and text.
    return node[0, 2] if node.first.is_a?(Symbol) && node.first.to_s.start_with?("@")

    node.map { |child| normalize(child) }
  end

  def self.install_definitions(node, found = [])
    return found unless node.is_a?(Array)

    found << node if node.first == :def && node[1][1] == "install"
    node.each { |child| install_definitions(child, found) }
    found
  end

  def self.reviewed?(name, text)
    return false if text.bytesize > 1024 * 1024

    parsed = Ripper.sexp(text)
    return false unless parsed

    definitions = install_definitions(parsed)
    return false unless definitions.length == 1

    digest = Digest::SHA256.hexdigest(JSON.generate(normalize(definitions.first)))
    REVIEWED_INSTALL[name] == digest
  end
end
