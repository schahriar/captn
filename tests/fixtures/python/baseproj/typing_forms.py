from typing import Annotated, Callable, List, Optional, Union


def maybe(a: Optional[int]) -> None:
    return None


def apply(fn: Callable[[int], str], v: Union[int, str]) -> str:
    return fn(1)


def tagged(a: Annotated[int, "x"], b: List[int]) -> None:
    return None


class Box[T]:
    def get(self, a: T) -> T:
        return a


class Pair[K, V]:
    def second(self, k: K, v: V) -> V:
        return v


def bounded[T: int](a: T) -> T:
    return a


type Count = int


def count(a: Count) -> Count:
    return a
