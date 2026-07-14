document.querySelector("#copy-btn-1").addEventListener("click", () => {
  const textarea = document.createElement("textarea");
  textarea.value = document.querySelector("#algorithm-1").innerText;
  document.body.appendChild(textarea);
  textarea.select();
  document.execCommand("copy");
  document.body.removeChild(textarea);
  alert("Copied to Clipboard!");
});