typealias Label = String

class Box<U> {
    func get(v: U) -> U {
        return v
    }
}

func ident<T>(v: T) -> T {
    return v
}

func label(v: Label) -> Label {
    return v
}

func v() -> Void {
}

func b(x: Bool, y: Double) {
}

func ranges(r: Range<Int>, c: ClosedRange<Int>, xs: [Int]) -> Int {
    let p = xs.prefix(2)
    let d = String(describing: p)
    for i in stride(from: 0, to: 10, by: 2) {
        _ = i
    }
    return d.count
}

func seq<S: Sequence>(s: S) -> Int where S.Element == Int {
    return s.underestimatedCount
}
