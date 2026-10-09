import { URLS } from "../../urls";

function TipLink() {
  return (
    <a
      className="text-xs underline text-gray-500 text-right"
      href={URLS.tips}
      target="_blank"
      rel="noopener"
    >

      提升生成效果的小技巧
    </a>
  );
}

export default TipLink;
