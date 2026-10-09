import { useState, useRef, useEffect } from "react";
import { Button } from "../ui/button";
import { Textarea } from "../ui/textarea";
import toast from "react-hot-toast";

interface GenerateFromTextProps {
  doCreateFromText: (text: string) => void;
}

function GenerateFromText({ doCreateFromText }: GenerateFromTextProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [text, setText] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (isOpen && textareaRef.current) {
      textareaRef.current.focus();
    }
  }, [isOpen]);

  const handleGenerate = () => {
    if (text.trim() === "") {
      toast.error("请输入生成要求");
      return;
    }
    doCreateFromText(text);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      handleGenerate();
    }
  };

  return (
    <div className="mt-4">
      {!isOpen ? (
        <div className="flex justify-center">
          <Button variant="secondary" onClick={() => setIsOpen(true)}>

            通过文字生成（测试版）
          </Button>
        </div>
      ) : (
        <>
          <Textarea
            ref={textareaRef}
            rows={2}
            placeholder="包含图表和用户管理的 SaaS 管理后台"
            className="w-full mb-4"
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={handleKeyDown}
          />
          <div className="flex justify-between items-center">
            <span className="text-sm text-gray-500">

              按 Cmd/Ctrl + Enter 生成
            </span>
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => setIsOpen(false)}>

                取消
              </Button>
              <Button onClick={handleGenerate}>生成</Button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

export default GenerateFromText;
