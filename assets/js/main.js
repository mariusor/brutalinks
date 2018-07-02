$( document ).ready(function() {

    var _User = JSON.parse($("#currentUser").html());
    console.debug(_User);

    var isInverted = Cookies.get("inverted") || false;

    if (isInverted) {
        $(":root").toggleClass("inverted");
    }
    $("#act-invert").click(function(e) {
        $(":root").toggleClass("inverted");
        if (isInverted) {
            Cookies.remove("inverted");
        } else {
            Cookies.set("inverted", true);
        }
        e.preventDefault();
        e.stopPropagation();
    });
});