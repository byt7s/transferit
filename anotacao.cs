// TRANSFERENCIA INTERNA
await iuguApi.Marketplace.TransferValueBetweenAccountsAsync(new TransferRequest
{
    AmountCents = (int)Math.Round(PaymentTaxes.PIX_TAXES_WITHDRAW_LASTLINK * 100),
    ReceiverId = LastlinkAccountId
}, token);

// NOVO USER
var response = await iuguApi.Marketplace.CreateAccountAsync(new NewAccountMarketplaceModel
{
    Name = identity.Name,                    
    Comissions = new ComissionsMarketplaceModel
    {
        Cents = commissionPlan.CommissionCents,
        Percent = commissionPlan.CommissionPercent.ToFloat(),
        CreditCardCents = commissionPlan.CommissionCents,
        CreditCardsPercent = commissionPlan.CommissionPercent.ToFloat(),
        BankSlipCents = commissionPlan.CommissionCentsBankSlip,
        BankSlipPercent = commissionPlan.CommissionPercentBankSlip.ToFloat(),
        PermitAggregated = true,
        PixCents = commissionPlan.CommissionCentsPix,
        PixPercent = commissionPlan.CommissionPercentPix.ToFloat()
    }
});

// PIX
var iuguResponse = await iuguApi.Marketplace.TransferToThirdPartiesAsync(new TransferToThirdPartiesModel
{
    ApiToken = token,
    TransferType = "pix",
    AmountCents = (int)(requestWithdraw.Amount * 100),
    CheckPayer = wallet.Identity.Document,
    Description = $"Saque Lastlink - {wallet.Identity.Name}",
    Receiver = new TransferToThirdPartiesReceiver
    {
        Pix = new TransferToThirdPartiesPix
        {
            Type = wallet.PixKeyType.Description,
            Key = wallet.PixKey
        }
    }
}, token);

// PEGAR COMPROVANTE (INUTIL, JA VEM NA RESPOSTA DO PIX)
var proof = await iuguApi.Marketplace.GetProofOfTransferToThirdParties(response.TransferRequestId, token);
