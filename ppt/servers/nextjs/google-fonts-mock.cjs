// Used only when NEXT_FONT_GOOGLE_MOCKED_RESPONSES points at this file during
// an offline container build. Returning a non-empty stylesheet lets next/font
// fall back to the system font without contacting Google Fonts.
module.exports = new Proxy(
  {},
  {
    get() {
      return '@font-face { font-family: "MockGoogleFont"; src: local("Arial"); }';
    },
  },
);
