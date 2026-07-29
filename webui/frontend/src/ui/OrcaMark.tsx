// ORCA-MARK — the product's own brand mark. Do NOT replace this with Codex's
// cloud-and-prompt glyph during a parity pass: the mark is the one place this
// product is deliberately NOT Codex, and reverting it is a regression, not an
// alignment. Parity work covers layout, type, spacing and colour; identity is
// out of its scope.
//
// It is a killer whale in side view, drawn to the animal's real proportions
// (blunt rounded head, dorsal fin at ~45% of the body and taller than the body
// is deep, thin caudal peduncle, notched fluke). Three passes of "render it,
// look at it, fix what is wrong" got it here; the earlier attempts all read as
// sharks or tuna because the head came to a point and the dorsal sat too far
// back.
//
// Two shapes are cut out of the body with `fill-rule="evenodd"` rather than
// painted white, so the mark inverts correctly: on a dark surface the body
// takes the light ink and the cutouts show the dark background through. The
// cutouts are the eye patch and the white jaw — the eye patch especially,
// since it is the one marking that separates an orca from any other whale.
export function OrcaMark({
  size = 24,
  className,
}: {
  size?: number;
  className?: string;
}) {
  return (
    <svg
      width={size}
      height={(size * 58) / 100}
      viewBox="0 0 100 58"
      className={className}
      aria-hidden
      focusable="false"
    >
      <path
        fill="currentColor"
        fillRule="evenodd"
        d="M8 29C8 23 12 18 20 15C28 12 36 10.5 44 10.5C47 7 50 3.5 54 0C55 6 56.5 11 59.5 15C67 18 76 24 83 31C85 33 86.2 35 87 37C90 33 94 30 99 27C96 32 94 36 93 40C95 44 96 49 97 55C93 51 89 46 86 42C81 42 75 41 68 39.5C65 38.9 62.5 38.4 60 37.9C64 43 68 48 71 54C65 47 58 42 51 36.6C44 35.8 36 34.6 28 33C18 30.6 10 29.6 8 29ZM19 17c4.5-1.6 9-.5 10 2.2c1 2.7-1.8 5.4-6.3 6.5c-4.5 1.1-9 0-10-2.7c-1-2.7 1.8-5.4 6.3-6ZM9 26.6C9 23.6 12 24.8 18 26.9C24 29 32 31.1 40 32.6C45 33.5 49 34.1 52 34.5C47 37.4 40 37.6 32 36.5C22 35.2 14 32.7 10 30C9.3 29.3 9 28.2 9 26.6Z"
      />
    </svg>
  );
}
