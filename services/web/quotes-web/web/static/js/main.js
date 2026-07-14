$(document).ready(function(){

    $('#allQuotes').on('click', function() {
            $.ajax({
		    url: 'https://api.thirdeye.live/quote',
                type: 'GET',
                data: 'json',
                success: function(response){
                    $("#randomQuoteData").text(JSON.stringify(response)); // XSS fix (Audit C-11): .text() never parses HTML
                },

                error: function() {
                    alert('request failed');
                }
            });
    });

    $('#allAuthors').on('click', function() {
            $.ajax({
                url: 'https://api.thirdeye.live/authors',
                type: 'GET',
                data: 'json',
                success: function(response){
                    $("#allAuthorsData").text(JSON.stringify(response)); // XSS fix (Audit C-11): .text() never parses HTML
                },

                error: function() {
                    alert('request failed');
                }
            });
    });

    $('#searchQuotes').on('click', function() {
        var formData = {
          words: $("#keywords").val(),
        };
            $.ajax({
                url: 'https://api.thirdeye.live/search',
                type: 'POST',
                data: formData,
                dataType: "json",
                encode: true,
                success: function(response){
                    $("#searchData").text(JSON.stringify(response)); // XSS fix (Audit C-11): .text() never parses HTML
                },

                error: function() {
                    alert('request failed');
                }
            });
    });

    $('#getQuote').on('click', function() {
        var id = $("#get-quote-id").val();
            $.ajax({
                url: 'https://api.thirdeye.live/quote/' + id.trim(),
                type: 'GET',
                data: "json",
                success: function(response){
                    $("#quoteByIdData").text(JSON.stringify(response)); // XSS fix (Audit C-11): .text() never parses HTML
                },

                error: function() {
                    alert('request failed');
                }
            });
    });

    $('#postQuote').on('click', function() {
        var token = $("#post-quote-auth").val();
            $.ajax({
                url: 'https://api.thirdeye.live/quote',
		headers: { 'Authorization': token.trim() },
                type: 'POST',
                data: JSON.stringify({
                    attribution: $("#post-quote-author").val(),
		    quote: $("#post-quote-text").val(),
		}),
                dataType: "json",
                contentType: "application/json",
                success: function(response){
                    $("#postQuoteReturnData").text(JSON.stringify(response)); // XSS fix (Audit C-11): .text() never parses HTML
                },

                error: function() {
                    alert('request failed');
                }
            });
    });

    $('#updateQuote').on('click', function() {
        var token = $("#update-quote-auth").val();
        var qid = $("#update-quote-id").val();
            $.ajax({
                url: 'https://api.thirdeye.live/quote/' + qid.trim(),
		headers: { 'Authorization': token.trim() },
                type: 'PUT',
                data: JSON.stringify({
                    id: qid.trim(),
		    attribution: $("#update-quote-author").val(),
	            quote: $("#update-quote-text").val(),
		}),
                dataType: "json",
                contentType: "application/json",
                success: function(response){
                    $("#updateQuoteReturnData").text(JSON.stringify(response)); // XSS fix (Audit C-11): .text() never parses HTML
                },

                error: function() {
                    alert('request failed');
                }
            });
    });

    $('#deleteQuote').on('click', function() {
        var id = $("#delete-quote-id").val();
        var token = $("#delete-quote-auth").val();
            $.ajax({
                url: 'https://api.thirdeye.live/quote/' + id.trim(),
		headers: { 'Authorization': token.trim() },
                type: 'DELETE',
                data: "json",
                success: function(response){
                    $("#deleteQuoteReturnData").text(JSON.stringify(response)); // XSS fix (Audit C-11): .text() never parses HTML
                },

                error: function() {
                    alert('request failed');
                }
            });
    });

    $('#getQuotesByAuthor').on('click', function() {
        var formData = {
          name: $("#get-quote-by-author").val(),
        };
            $.ajax({
                url: 'https://api.thirdeye.live/author-quotes',
                type: 'POST',
                data: formData,
                dataType: "json",
                encode: true,
                success: function(response){
                    $("#quotesByAuthorData").text(JSON.stringify(response)); // XSS fix (Audit C-11): .text() never parses HTML
                },

                error: function() {
                    alert('request failed');
                }
            });
    });

});
