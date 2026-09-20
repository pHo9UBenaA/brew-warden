# Inspect a bounded build-language subset; never evaluate recipe expressions.
require "ripper"

module BrewWardenSource
  class Unsupported < StandardError; end
  PATHS = %w[prefix bin sbin lib libexec include share man etc var rpath].freeze
  STANDARD_ARGS = %w[std_configure_args std_cmake_args].freeze

  def self.install_definitions(node, found = [])
    return found unless node.is_a?(Array)
    found << node if node.first == :def && node[1][1] == "install"
    node.each { |child| install_definitions(child, found) }
    found
  end

  def self.reviewed?(text)
    return false if text.bytesize > 1024 * 1024
    parsed = Ripper.sexp(text)
    return false unless parsed
    definitions = install_definitions(parsed)
    return false unless definitions.length == 1
    definition = definitions.first
    return false unless definition[2] == [:params, nil, nil, nil, nil, nil, nil, nil]
    body = definition[3]
    return false unless body[0] == :bodystmt && body[2..].all?(&:nil?) && body[1].length <= 128
    locals = {}
    commands = []
    body[1].each do |statement|
      case statement[0]
      when :assign
        field = statement[1]
        raise Unsupported unless field[0] == :var_field && field[1][0] == :@ident
        name = field[1][1]
        raise Unsupported if locals.key?(name) || (PATHS + STANDARD_ARGS).include?(name)
        locals[name] = value(statement[2], locals)
      when :if_mod
        # Stable bottles do not execute an explicitly head-only build branch.
        condition = statement[1]
        raise Unsupported unless condition[0] == :call && condition[1][0] == :vcall && condition[1][1][1] == "build" && condition[3][1] == "head?"
      else
        raise Unsupported unless statement[0] == :command && statement[1][1] == "system"
        args = statement[2]
        raise Unsupported unless args[0] == :args_add_block && args[2] == false
        command = arguments(args[1], locals)
        raise Unsupported unless supported_command?(command)
        commands << command
      end
    end
    # An empty method or only local assignments does not establish a build.
    !commands.empty?
  rescue Unsupported, SystemStackError
    false
  end

  def self.arguments(node, locals)
    result = if node[0] == :args_add_star
      star = value(node[2], locals)
      raise Unsupported unless star.is_a?(Array)
      arguments(node[1], locals) + star + node[3..].map { |part| value(part, locals) }
    else
      raise Unsupported unless node.is_a?(Array) && node.all? { |part| part.is_a?(Array) }
      node.map { |part| value(part, locals) }
    end
    raise Unsupported if result.length > 256
    result
  end

  def self.value(node, locals)
    raise Unsupported unless node.is_a?(Array)
    case node[0]
    when :string_literal
      raise Unsupported unless node[1][0] == :string_content
      string(node[1][1..], locals)
    when :array
      raise Unsupported unless node[1].is_a?(Array) && node[1].length <= 256
      node[1].map { |part| part[0].is_a?(Array) ? string(part, locals) : value(part, locals) }
    when :vcall
      name = node[1][1]
      return ["@#{name}"] if STANDARD_ARGS.include?(name)
      raise Unsupported unless PATHS.include?(name)
      "@#{name}"
    when :var_ref
      raise Unsupported unless node[1][0] == :@ident && locals.key?(node[1][1])
      locals.fetch(node[1][1])
    else
      raise Unsupported
    end
  end

  def self.string(parts, locals)
    result = parts.map do |part|
      case part[0]
      when :@tstring_content then part[1]
      when :string_embexpr
        raise Unsupported unless part[1].length == 1
        result = value(part[1][0], locals)
        raise Unsupported unless result.is_a?(String)
        result
      else raise Unsupported
      end
    end.join
    raise Unsupported if result.length > 4096
    result
  end

  def self.supported_command?(args)
    return false unless args.all? { |arg| arg.is_a?(String) && arg.match?(/\A[A-Za-z0-9_@.\/+,:=-]+\z/) }
    command, *flags = args
    case command
    when "autoreconf"
      (flags - %w[--force --install --verbose -f -i -v]).empty?
    when "./configure"
      flags.all? { |arg| arg == "@std_configure_args" || arg.match?(/\A--[a-z][a-z0-9-]*(?:=[A-Za-z0-9_@.\/+,:=-]+)?\z/) }
    when "make"
      (flags - %w[all install check test]).empty?
    when "cmake"
      return true if [%w[--build build], %w[--install build]].include?(flags)
      return false unless flags[0, 4] == %w[-S . -B build]
      flags[4..].all? do |arg|
        arg == "@std_cmake_args" || (arg.match?(/\A-D[A-Z][A-Z0-9_]*(?::BOOL)?=[A-Za-z0-9_@.\/+,:=-]+\z/) && !arg.match?(/\A-DCMAKE_.*(?:INCLUDE|TOOLCHAIN|COMPILER|FLAGS|COMMAND|RULES)/))
      end
    else false
    end
  end
end
