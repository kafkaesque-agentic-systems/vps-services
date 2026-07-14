// This example uses an external library for checking resize of the element.
var container = document.getElementById("code-block");
container.addEventListener("onresize", setFontSize);

function setFontSize(){
  var width = container.offsetWidth; // Get the width of the div

  var minSize = 11; // Minimum font size

  // Here, we make any operation on the width/height and check if it is less that the minimum font size, or any other conditions.
  var toSet = (width * 0.03) < minSize ? minSize : (width * 0.03);

  document.getElementById("algo-font").style.fontSize = toSet + "px";
}

setFontSize();