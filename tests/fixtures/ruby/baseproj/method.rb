class Widget
  attr_reader :label

  def initialize(label)
    @label = label
  end

  def describe(prefix)
    format("%s: %s", prefix, @label)
  end
end
