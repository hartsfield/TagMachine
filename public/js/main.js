var tags = [];
if (window.location.href.includes("tags")) {
    tags = (window.location.search.substring(window.location.search.indexOf('=')).substring(1).split(",").filter(v => v));
    if (window.location.search.length > 0 && tags.length <= 0 && !window.location.href.includes("Num=")) {
        window.location = window.location.origin;
    }
}

var loc = window.location.pathname.split("/");
var uname = loc[loc.length - 1];

var isTrendshowing = false;

function showTrending() {
    if (isTrendshowing == false) {
        document.getElementById("showTags").innerHTML = "[LESS]";
        var elms = document.getElementsByClassName("trending");
        for (var i = 0; i < elms.length; i++) {
            elms[i].style.display = 'inline-flex';
        }
        isTrendshowing = true;
    } else {
        document.getElementById("showTags").innerHTML = "TRENDS ➡";
        var elms2 = document.getElementsByClassName("trending");
        for (var k = 0; k < elms2.length; k++) {
            elms2[k].style.display = 'none';
        }

        isTrendshowing = false;
    }
}

function showLogin() {
    if (document.getElementById("loginForm").style.display == "none") {
        document.getElementById("loginForm").style.display = "inline-flex";
    } else {
        document.getElementById("loginForm").style.display = "none";
    }
}

function showPostForm() {
    if (document.getElementById("newPostForm").style.display == "none") {
        document.getElementById("newPostForm").style.display = "block";
        if (tags.length >= 1) {
            document.getElementById("taginput").value = "#" + tags.join(", #");
        }
    } else {
        document.getElementById("newPostForm").style.display = "none";
        document.getElementById("taginput").value = "";
    }
}

function showReplyForm(id) {
    if (document.getElementById("replyForm_" + id).style.display == "none") {
        document.getElementById("replyForm_" + id).style.display = "unset";
        document.getElementById("showReplyFormButt_" + id).innerHTML = "- REPLY";
    } else {
        document.getElementById("replyForm_" + id).style.display = "none";
        document.getElementById("showReplyFormButt_" + id).innerHTML = "+ REPLY";
    }
}

function auth(path) {
    var xhr = new XMLHttpRequest();

    xhr.open('POST', '/api/' + path);
    xhr.setRequestHeader('Content-Type', 'application/json');
    xhr.onload = function() {
        if (xhr.status === 200) {
            console.log(xhr.responseText);
            var res = JSON.parse(xhr.responseText);
            if (res.success == "false") {
                console.log("ERROR", res);
                document.getElementById("errorField").innerHTML = res.error;
            } else {
                window.location.reload();
            }
        }
    };

    xhr.send(JSON.stringify({
        password: document.getElementById("password").value,
        username: document.getElementById("username").value,
    }));
}


function reply(id, parent) {
    console.log("reply");
    var xhr = new XMLHttpRequest();

    xhr.open('POST', '/api/reply');
    xhr.setRequestHeader('Content-Type', 'application/json');
    xhr.onload = function() {
        if (xhr.status === 200) {
            var res = JSON.parse(xhr.responseText);
            if (res.success == "true") {
                window.location = window.location.origin + "/view/?postNum=" + id;
            } else {
                document.getElementById("replyError").innerHTML = "Error: Are you still logged in?"
            }
        }
    };

    xhr.send(JSON.stringify({
        body: document.getElementById("replyBody_" + id).value,
        ID: id,
        parent: parent,
    }));
}

function replyThread(id, parent, pTags) {
    console.log(id, parent, pTags);
    var xhr = new XMLHttpRequest();

    xhr.open('POST', '/api/reply');
    xhr.setRequestHeader('Content-Type', 'application/json');
    xhr.onload = function() {
        if (xhr.status === 200) {
            var res = JSON.parse(xhr.responseText);
            if (res.success == "true") {
                window.location = window.location.origin + "/view/?postNum=" + id;
            } else {
                document.getElementById("replyError").innerHTML = "Error: Are you still logged in?"
            }
        }
    };

    xhr.send(JSON.stringify({
        body: document.getElementById("replyBody_" + id).value,
        ID: id,
        Tags: pTags,
        parent: parent,
    }));
}

function setTag(tag) {
    if (tags.includes(tag)) {
        tags = tags.filter(x => x !== tag);
        window.location = window.location.origin + "/tag/?tags=" + tags.join(",");
        document.getElementById("tag_" + tag).style.backgroundColor = "#d52e12";
        // document.getElementById("tag_" + tag).style.backgroundColor = "rgb(33 169 162)";
    } else {
        tags.push(tag);
        window.location = window.location.origin + "/tag/?tags=" + tags.join(",");
        // document.getElementById("tag_" + tag).style.backgroundColor = "#91ba03";
        document.getElementById("tag_" + tag).style.backgroundColor = "rgb(145, 186, 3)";
    }
    console.log(tags);
}

function postThread(id) {
    var titlearea = document.getElementById("postTitle").value;
    var textarea = document.getElementById("postBody").value;
    var tagarea = document.getElementById("taginput").value;
    if (tagarea.length <= 3 || textarea.length <= 5 || titlearea.length <= 2) {
        document.getElementById("postButt").innerHTML = "No input field can be blank";
    } else {
        var xhr = new XMLHttpRequest();

        xhr.open('POST', '/api/newthread');
        xhr.setRequestHeader('Content-Type', 'application/json');
        xhr.onload = function() {
            if (xhr.status === 200) {
                console.log(xhr.responseText);
                var res = JSON.parse(xhr.responseText);
                if (res.success == "true") {
                    document.getElementById("newPostForm").style.display = "none";
                    window.location = window.location.origin + "/view/?postNum=" + res.postID;
                } else {
                    document.getElementById("postButt").innerHTML = "error";
                }
            }
        };
        var newTags = tagarea.match(/[^\s,]+/g);
        console.log(newTags, newTags[0]);
        xhr.send(JSON.stringify({
            title: document.getElementById("postTitle").value,
            body: document.getElementById("postBody").value,
            tags: newTags,
        }));
    }
}

function viewContext(parentID) {
    window.location = window.location.origin + "/view/?postNum=" + parentID;
}

function viewUser(user) {
    window.location = window.location.origin + "/user/" + user;
}
